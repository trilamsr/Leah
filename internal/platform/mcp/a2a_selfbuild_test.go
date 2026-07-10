package mcp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/trilam/leah/internal/platform/audit"
	"github.com/trilam/leah/internal/platform/testutil"
)

// stubRunner records intents Run was called with; returns configured err.
// When clarifyText is set, Run writes it to out before returning runErr (a
// dispatcher.ErrSelfBuildClarify analog under ErrSelfBuildClarify).
type stubRunner struct {
	mu          sync.Mutex
	intents     []string
	runErr      error
	runDelay    time.Duration
	clarifyText string
}

func (r *stubRunner) Run(ctx context.Context, intent string, out io.Writer) error {
	if r.runDelay > 0 {
		time.Sleep(r.runDelay)
	}
	r.mu.Lock()
	r.intents = append(r.intents, intent)
	err := r.runErr
	clarify := r.clarifyText
	r.mu.Unlock()
	if clarify != "" {
		_, _ = io.WriteString(out, clarify)
	}
	return err
}

func (r *stubRunner) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := append([]string(nil), r.intents...)
	return out
}

// stubTTY answers the attestation prompt with a fixed verdict.
type stubTTY struct {
	ok      bool
	delayed time.Duration
	mu      sync.Mutex
	calls   int
}

func (s *stubTTY) Confirm(ctx context.Context, peer, tool, args, question string) bool {
	if s.delayed > 0 {
		select {
		case <-time.After(s.delayed):
		case <-ctx.Done():
			return false
		}
	}
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	return s.ok
}

func writeFile(path, body string) error { return os.WriteFile(path, []byte(body), 0o600) }

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

func newA2AServer(t *testing.T, tty TTYConfirmer, runner SelfBuildRunner) (*Server, *audit.Logger, string) {
	t.Helper()
	s, logger, dir := newTestServer(t)
	// Pool with a single question registered for the a2a scope.
	qPath := filepath.Join(dir, "q.txt")
	if err := writeFile(qPath, "What is the smallest change that closes this?\n"); err != nil {
		t.Fatal(err)
	}
	s.A2A = &A2AHandler{
		AgentCardPath:            filepath.Join(dir, "agent_card.json"),
		AuditLogger:              logger,
		AttestationTTY:           tty,
		SelfBuild:                runner,
		AttestationQuestionsPath: qPath,
		AttestationTimeout:       2 * time.Second,
	}
	return s, logger, dir
}

func postTask(t *testing.T, addr, token, body string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest("POST", "http://"+addr+"/a2a/tasks",
		bytes.NewReader([]byte(body)))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// Without bearer → 401 (server-level auth). With bearer + missing attestation
// (stubTTY returns false) → 403 + mcp_attestation_denied audit row.
func TestA2ASelfBuild_RequiresAttestation(t *testing.T) {
	tty := &stubTTY{ok: false}
	runner := &stubRunner{}
	s, logger, _ := newA2AServer(t, tty, runner)
	cancel := startListener(t, s)
	defer cancel()

	code, _ := postTask(t, s.Addr, "", `{"skill":"self_build","intent":"x"}`)
	if code != http.StatusUnauthorized {
		t.Errorf("no-bearer want 401, got %d", code)
	}

	code, _ = postTask(t, s.Addr, testToken, `{"skill":"self_build","intent":"x"}`)
	if code != http.StatusForbidden {
		t.Errorf("attestation-deny want 403, got %d", code)
	}
	if got := runner.snapshot(); len(got) != 0 {
		t.Errorf("runner must not run on deny, got %v", got)
	}
	testutil.Eventually(t, time.Second, 5*time.Millisecond, func() bool {
		return strings.Contains(readFile(t, logger.Path), `"kind":"mcp_attestation_denied"`)
	})
}

// 1 attestation prompt / min per peer (spec §2.3). Two rapid posts → 1 prompt
// + 1 429 mcp_attestation_rate_limited.
func TestA2ASelfBuild_RateLimits_1PerMin(t *testing.T) {
	tty := &stubTTY{ok: true, delayed: 100 * time.Millisecond}
	runner := &stubRunner{}
	s, logger, _ := newA2AServer(t, tty, runner)
	// Disable server's read-path rate limit so we isolate attestation 1/min.
	s.RatePerMin = 1000
	cancel := startListener(t, s)
	defer cancel()

	// First in-flight; second arrives while first still prompting →
	// rate-limit triggers (per-peer 1/min).
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		postTask(t, s.Addr, testToken, `{"skill":"self_build","intent":"a"}`)
	}()
	time.Sleep(20 * time.Millisecond)
	code, _ := postTask(t, s.Addr, testToken, `{"skill":"self_build","intent":"b"}`)
	if code != http.StatusTooManyRequests {
		t.Errorf("2nd call want 429, got %d", code)
	}
	wg.Wait()
	raw := readFile(t, logger.Path)
	if !strings.Contains(raw, `"kind":"mcp_attestation_rate_limited"`) {
		t.Errorf("want mcp_attestation_rate_limited audit row, got %s", raw)
	}
}

// Global pending cap = 5 (spec §2.3). 6th concurrent in-flight self_build →
// 503 + mcp_attestation_queue_full cap=global.
func TestA2ASelfBuild_PendingCap5(t *testing.T) {
	tty := &stubTTY{ok: true, delayed: 500 * time.Millisecond}
	runner := &stubRunner{}
	s, logger, _ := newA2AServer(t, tty, runner)
	s.RatePerMin = 1000
	// Relax per-peer attestation 1/min so all 5 calls (same token) count toward global cap.
	s.A2A.AttestationRatePerMin = 1000
	cancel := startListener(t, s)
	defer cancel()

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			postTask(t, s.Addr, testToken, `{"skill":"self_build","intent":"x"}`)
		}()
	}
	// Wait for all 5 to enter pending (TTY still sleeping).
	testutil.Eventually(t, time.Second, 5*time.Millisecond, func() bool {
		return s.PendingGlobalCount() >= 5
	})
	code, _ := postTask(t, s.Addr, testToken, `{"skill":"self_build","intent":"x"}`)
	if code != http.StatusServiceUnavailable {
		t.Errorf("6th want 503, got %d", code)
	}
	wg.Wait()
	raw := readFile(t, logger.Path)
	if !strings.Contains(raw, `"kind":"mcp_attestation_queue_full"`) {
		t.Errorf("want mcp_attestation_queue_full audit, got %s", raw)
	}
	if !strings.Contains(raw, "cap=global") {
		t.Errorf("want cap=global, got %s", raw)
	}
}

// Happy path: bearer + TTY ok → dispatcher.SelfBuild.Run called with intent.
func TestA2ASelfBuild_TTYAttestationFlow(t *testing.T) {
	tty := &stubTTY{ok: true}
	runner := &stubRunner{}
	s, logger, _ := newA2AServer(t, tty, runner)
	cancel := startListener(t, s)
	defer cancel()

	code, body := postTask(t, s.Addr, testToken,
		`{"skill":"self_build","intent":"add /leah whoami fast-path"}`)
	if code != http.StatusAccepted {
		t.Fatalf("want 202, got %d body=%s", code, body)
	}
	// Runner ran synchronously before the HTTP response returned.
	got := runner.snapshot()
	if len(got) != 1 || got[0] != "add /leah whoami fast-path" {
		t.Fatalf("runner intents = %v, want single intent", got)
	}
	raw := readFile(t, logger.Path)
	if !strings.Contains(raw, `"kind":"mcp_call"`) {
		t.Errorf("want mcp_call audit, got %s", raw)
	}
	if !strings.Contains(raw, "tool=self_build") {
		t.Errorf("want tool=self_build in detail, got %s", raw)
	}
	if !strings.Contains(raw, "attest=true") {
		t.Errorf("want attest=true marker, got %s", raw)
	}
}

// Malformed body → mcp_malformed_task + 400.
func TestA2ASelfBuild_MalformedTask(t *testing.T) {
	tty := &stubTTY{ok: true}
	runner := &stubRunner{}
	s, logger, _ := newA2AServer(t, tty, runner)
	cancel := startListener(t, s)
	defer cancel()

	code, _ := postTask(t, s.Addr, testToken, `{not-json`)
	if code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", code)
	}
	raw := readFile(t, logger.Path)
	if !strings.Contains(raw, `"kind":"mcp_malformed_task"`) {
		t.Errorf("want mcp_malformed_task audit, got %s", raw)
	}
}

// Spec §5 clarify branch: reasoner returns clarifying questions → 403 +
// clarify_required body. No PR is filed.
func TestA2A_ReasonerClarify_Returns_ClarifyRequired_NoPR(t *testing.T) {
	tty := &stubTTY{ok: true}
	runner := &stubRunner{runErr: ErrSelfBuildClarify, clarifyText: "Which repo? Which branch?"}
	s, logger, _ := newA2AServer(t, tty, runner)
	cancel := startListener(t, s)
	defer cancel()

	code, body := postTask(t, s.Addr, testToken,
		`{"skill":"self_build","intent":"x"}`)
	if code != http.StatusForbidden {
		t.Fatalf("want 403, got %d body=%s", code, body)
	}
	if !strings.Contains(body, `"status":"clarify_required"`) {
		t.Errorf("want clarify_required in body, got %s", body)
	}
	if !strings.Contains(body, "Which repo?") {
		t.Errorf("want clarify text in body, got %s", body)
	}
	// Runner was invoked (reasoner ran) but no PR side-effects beyond that.
	if got := runner.snapshot(); len(got) != 1 {
		t.Errorf("runner should be called once, got %v", got)
	}
	raw := readFile(t, logger.Path)
	if !strings.Contains(raw, `"outcome":"clarify"`) {
		t.Errorf("want clarify audit outcome, got %s", raw)
	}
}

// Spec §2.3 fail-closed: no attestation pool configured → 503, gate cannot be
// silently bypassed by a missing config.
func TestA2A_AttestationPoolUnconfigured_FailsClosed(t *testing.T) {
	tty := &stubTTY{ok: true}
	runner := &stubRunner{}
	s, _, dir := newTestServer(t)
	s.A2A = &A2AHandler{
		AgentCardPath:  filepath.Join(dir, "agent_card.json"),
		AuditLogger:    s.AuditLogger,
		AttestationTTY: tty,
		SelfBuild:      runner,
		// AttestationQuestionsPath intentionally left empty.
		AttestationTimeout: 2 * time.Second,
	}
	cancel := startListener(t, s)
	defer cancel()

	code, _ := postTask(t, s.Addr, testToken, `{"skill":"self_build","intent":"x"}`)
	if code != http.StatusServiceUnavailable {
		t.Errorf("want 503, got %d", code)
	}
	if got := runner.snapshot(); len(got) != 0 {
		t.Errorf("runner must not run when pool unconfigured, got %v", got)
	}
}

// Spec §6 throttle: identical-tamper mcp_card_reset rows suppressed within 60s.
func TestA2ACard_TamperFlood_SuppressedWithinWindow(t *testing.T) {
	s, logger, dir := newTestServer(t)
	cardPath := filepath.Join(dir, "agent_card.json")
	tampered := `{"securitySchemes":{}}`
	if err := writeFile(cardPath, tampered); err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 0)
	s.A2A = &A2AHandler{
		AgentCardPath:  cardPath,
		AuditLogger:    logger,
		AttestationTTY: &stubTTY{ok: true},
		Now:            func() time.Time { return now },
	}
	cancel := startListener(t, s)
	defer cancel()

	for i := 0; i < 5; i++ {
		resp, err := http.Get("http://" + s.Addr + "/.well-known/agent-card.json")
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
	}
	raw := readFile(t, logger.Path)
	if strings.Count(raw, `"kind":"mcp_card_reset"`) != 1 {
		t.Errorf("want exactly 1 mcp_card_reset (suppressed flood), got:\n%s", raw)
	}
}

// Runner-error → 500 + slot released for subsequent calls. Guarded against deadlock.
func TestA2ASelfBuild_RunnerError_ReleasesSlot(t *testing.T) {
	tty := &stubTTY{ok: true}
	runner := &stubRunner{runErr: errors.New("boom")}
	s, _, _ := newA2AServer(t, tty, runner)
	cancel := startListener(t, s)
	defer cancel()

	code, _ := postTask(t, s.Addr, testToken, `{"skill":"self_build","intent":"x"}`)
	if code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", code)
	}
	if got := s.PendingGlobalCount(); got != 0 {
		t.Fatalf("pending slot not released after sync run, got %d", got)
	}
}
