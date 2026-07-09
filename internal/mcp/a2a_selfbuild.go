package mcp

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/trilam/leah/internal/attestation"
	"github.com/trilam/leah/internal/audit"
)

// ErrSelfBuildClarify signals the reasoner returned clarifying questions
// instead of a spec. A2A surfaces it as 403 + clarify_required without filing
// a PR. Production wiring translates dispatcher.ErrSelfBuildClarify into this
// sentinel via the runner adapter.
var ErrSelfBuildClarify = errors.New("mcp: reasoner requested clarification")

// ErrAttestationPoolUnconfigured is returned when the A2A handler is invoked
// without an attestation questions pool wired. Fail-closed (503): an unconfigured
// pool would silently bypass the operator-attestation gate.
var ErrAttestationPoolUnconfigured = errors.New("mcp: attestation question pool not configured")

// SelfBuildRunner is the dispatcher.SelfBuild contract from the A2A layer.
// Imported as an interface to keep internal/mcp file-disjoint from
// internal/dispatcher. out captures the reasoner's clarify text when Run
// returns ErrSelfBuildClarify.
type SelfBuildRunner interface {
	Run(ctx context.Context, intent string, out io.Writer) error
}

// TTYConfirmer prompts the operator via /dev/tty and returns true on accept,
// false on deny / timeout. HUD popup-confirm is a follow-up; v1 is TTY-only.
type TTYConfirmer interface {
	Confirm(ctx context.Context, peer, tool, args, question string) bool
}

// A2AHandler hosts the A2A 1.0 agent-card + SelfBuild task endpoint.
// Nil-safe: a Server with A2A == nil does not register the A2A routes.
type A2AHandler struct {
	AgentCardPath            string
	AuditLogger              *audit.Logger
	AttestationTTY           TTYConfirmer
	SelfBuild                SelfBuildRunner
	AttestationQuestionsPath string
	AttestationTimeout       time.Duration
	// AttestationRatePerMin overrides the spec §2.3 1/min per-peer cap
	// (default 1). Test-only escape hatch — production wiring leaves it 0.
	AttestationRatePerMin int

	// Now returns the current time. Test-injectable; defaults to time.Now.
	Now func() time.Time

	rateMu sync.Mutex
	// peer-id → rolling timestamps of recent prompt starts. Bounded by window.
	rate map[string][]time.Time

	// tamper throttles identical mcp_card_reset emissions (60s window).
	tamper tamperThrottle
}

func (a *A2AHandler) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now()
}

type a2aTask struct {
	Skill  string `json:"skill"`
	Intent string `json:"intent"`
}

type a2aTaskResp struct {
	TaskID string `json:"task_id"`
	Status string `json:"status"`
}

// PendingGlobalCount exposes the count of in-flight write tasks for tests +
// observability. Reads the Server semaphore through a shared lock.
func (s *Server) PendingGlobalCount() int {
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	return s.pendingG
}

func newTaskID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func (a *A2AHandler) allowAttestationPrompt(peer string, now time.Time) bool {
	cap := a.AttestationRatePerMin
	if cap == 0 {
		cap = 1
	}
	a.rateMu.Lock()
	defer a.rateMu.Unlock()
	if a.rate == nil {
		a.rate = map[string][]time.Time{}
	}
	cutoff := now.Add(-time.Minute)
	hist := a.rate[peer]
	kept := hist[:0]
	for _, t := range hist {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= cap {
		a.rate[peer] = kept
		return false
	}
	a.rate[peer] = append(kept, now)
	return true
}

func (a *A2AHandler) auditRow(kind, detail string, br int, outcome string) {
	if a.AuditLogger == nil {
		return
	}
	_ = a.AuditLogger.Append(auditEntry(kind, detail, br, outcome))
}

func auditEntry(kind, detail string, br int, outcome string) audit.Entry {
	return audit.Entry{Kind: kind, Detail: detail, BlastRadius: br, Outcome: outcome}
}

// handleA2ATask is the POST /a2a/tasks endpoint. Server-level bearer auth +
// 60/min read-rate already ran before this point (mcp_call ok row will be
// emitted by us, not the server.handle path).
func (s *Server) handleA2ATask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	a := s.A2A
	tok := bearerFrom(r)
	if !s.tokenMatches(tok) {
		s.audit("mcp_auth_fail", "peer="+peerHash(tok), 0, "denied")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	peer := peerHash(tok)

	body, err := readBody(r)
	if err != nil {
		a.auditRow("mcp_malformed_task", "peer="+peer+" err="+err.Error(), 0, "rejected")
		http.Error(w, "body too large", http.StatusBadRequest)
		return
	}
	var task a2aTask
	if err := json.Unmarshal(body, &task); err != nil {
		a.auditRow("mcp_malformed_task", "peer="+peer+" err=unmarshal", 0, "rejected")
		http.Error(w, "malformed task", http.StatusBadRequest)
		return
	}
	if task.Skill != "self_build" {
		a.auditRow("mcp_malformed_task", "peer="+peer+" err=unknown_skill", 0, "rejected")
		http.Error(w, "unknown skill", http.StatusBadRequest)
		return
	}

	// Per-peer 1/min attestation rate (separate from server's 60/min read rate).
	if !a.allowAttestationPrompt(peer, s.now()) {
		a.auditRow("mcp_attestation_rate_limited", "peer="+peer, 0, "rate_limited")
		http.Error(w, "attestation rate limited", http.StatusTooManyRequests)
		return
	}

	release, ok, scope := s.EnqueuePending(peer)
	if !ok {
		a.auditRow("mcp_attestation_queue_full", "peer="+peer+" cap="+scope, 0, "rejected")
		http.Error(w, "queue full", http.StatusServiceUnavailable)
		return
	}

	taskID := newTaskID()
	question, qErr := a.pickQuestion()
	if qErr != nil {
		release()
		a.auditRow("mcp_call", "peer="+peer+" tool=self_build err=attestation:"+qErr.Error(), 4, "failed")
		http.Error(w, "attestation pool unavailable", http.StatusServiceUnavailable)
		return
	}

	// Synchronous flow: TTY attestation → reasoner via SelfBuild.Run → HTTP
	// response carries the terminal verdict. The task-result callback path
	// (A2A 1.0 §6.3) is deferred alongside the HUD popup-confirm.
	timeout := a.AttestationTimeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	argsSummary := truncate(task.Intent, 80)
	accepted := false
	if a.AttestationTTY != nil {
		accepted = a.AttestationTTY.Confirm(ctx, peer, "self_build", argsSummary, question)
	}
	if !accepted {
		release()
		a.auditRow("mcp_attestation_denied", "peer="+peer+" task="+taskID, 3, "denied")
		http.Error(w, "attestation failed", http.StatusForbidden)
		return
	}

	a.auditRow("mcp_call",
		"peer="+peer+" tool=self_build attest=true task="+taskID, 4, "ok")

	// Run synchronously so a clarify return can be surfaced as 403 + clarify_required
	// (clarify → no PR filed). Reasoner+gh latency holds the HTTP request open;
	// acceptable for loopback v1, revisited when callback URLs ship.
	defer release()
	if a.SelfBuild == nil {
		a.auditRow("mcp_call", "peer="+peer+" tool=self_build task="+taskID+" err=no_runner", 4, "failed")
		http.Error(w, "self-build runner not configured", http.StatusServiceUnavailable)
		return
	}
	var clarifyBuf bytes.Buffer
	runErr := a.SelfBuild.Run(ctx, task.Intent, &clarifyBuf)
	switch {
	case errors.Is(runErr, ErrSelfBuildClarify):
		a.auditRow("mcp_call",
			"peer="+peer+" tool=self_build task="+taskID+" clarify", 4, "clarify")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"task_id": taskID,
			"status":  "clarify_required",
			"body":    clarifyBuf.String(),
		})
	case runErr != nil:
		a.auditRow("mcp_call",
			"peer="+peer+" tool=self_build task="+taskID+" err="+runErr.Error(), 4, "failed")
		http.Error(w, "self-build failed", http.StatusInternalServerError)
	default:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(a2aTaskResp{TaskID: taskID, Status: "dispatched"})
	}
}

// pickQuestion draws from the attestation pool against ScopeSelfBuildA2A.
// Fail-closed: returns ErrAttestationPoolUnconfigured when no path is set so
// the gate cannot be bypassed by a missing config (spec §2.3 anti-bypass).
func (a *A2AHandler) pickQuestion() (string, error) {
	if a.AttestationQuestionsPath == "" {
		return "", ErrAttestationPoolUnconfigured
	}
	pool, err := attestation.Load(a.AttestationQuestionsPath, attestation.AllScopes()...)
	if err != nil {
		return "", err
	}
	return pool.Pick(attestation.ScopeSelfBuildA2A)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
