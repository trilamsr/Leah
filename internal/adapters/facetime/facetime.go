package facetime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/trilam/leah/internal/obs/connectadapter"
)

var (
	ErrAttestationDenied = errors.New("facetime: attestation denied")
	ErrInvalidCallee     = errors.New("facetime: invalid callee")
	ErrRateLimited       = errors.New("facetime: rate limit exceeded")
	ErrLaunchFailed      = errors.New("facetime: open(1) failed")
)

const (
	ScopeInitiateVideo = "facetime:video"
	ScopeInitiateAudio = "facetime:audio"
)

const (
	rateWindow = time.Minute
	rateMax    = 5
)

var calleeRE = regexp.MustCompile(`^[+\w@.-]+$`)

type Attestor interface {
	Attest(ctx context.Context, scope string) error
}

type OSExec interface {
	Run(ctx context.Context, name string, args []string, stdin []byte) ([]byte, error)
}

type AuditRow struct {
	Kind       string `json:"kind"`
	Timestamp  string `json:"ts"`
	Mode       string `json:"mode"`
	Success    bool   `json:"success"`
	CalleeHash string `json:"callee_hash"`
	Reason     string `json:"reason,omitempty"`
}

type Sink interface {
	Audit(row AuditRow)
}

type noopSink struct{}

func (noopSink) Audit(AuditRow) {}

type Config struct {
	Attestor Attestor
	OSExec   OSExec
	Sink     Sink
	Now      func() time.Time
	// Metrics is optional — nil is a no-op (connectadapter contract), so
	// existing callers keep working without a registry.
	Metrics *connectadapter.Metrics
}

type Adapter struct {
	att  Attestor
	ex   OSExec
	sink Sink
	now  func() time.Time
	m    *connectadapter.Metrics

	mu     sync.Mutex
	bucket []time.Time
}

func New(cfg Config) (*Adapter, error) {
	if cfg.Attestor == nil {
		return nil, errors.New("facetime: Config.Attestor required (operator-attestation gate)")
	}
	if cfg.OSExec == nil {
		return nil, errors.New("facetime: Config.OSExec required")
	}
	sink := cfg.Sink
	if sink == nil {
		sink = noopSink{}
	}
	now := cfg.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Adapter{att: cfg.Attestor, ex: cfg.OSExec, sink: sink, now: now, m: cfg.Metrics}, nil
}

func (a *Adapter) InitiateVideo(ctx context.Context, callee string) error {
	return a.initiate(ctx, callee, "video", ScopeInitiateVideo, "facetime://", "initiate_video")
}

func (a *Adapter) InitiateAudio(ctx context.Context, callee string) error {
	return a.initiate(ctx, callee, "audio", ScopeInitiateAudio, "facetime-audio://", "initiate_audio")
}

// Order: validate -> rate-limit -> attest -> exec -> audit. Rate-limit must
// precede attestation so a compromised Leah cannot flood consent prompts.
// ObserveAPI wraps only the exec leg — that is the actual RPC; rejecting
// earlier (validation, rate-limit, denied attestation) is not wire-time.
func (a *Adapter) initiate(ctx context.Context, callee, mode, scope, scheme, endpoint string) error {
	if !calleeRE.MatchString(callee) {
		return fmt.Errorf("%w: %q", ErrInvalidCallee, callee)
	}
	if !a.allow() {
		a.audit(mode, false, callee, "rate limited")
		return ErrRateLimited
	}
	if err := a.att.Attest(ctx, scope); err != nil {
		a.audit(mode, false, callee, "attestation denied")
		return fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	start := time.Now()
	_, err := a.ex.Run(ctx, "open", []string{scheme + callee}, nil)
	a.m.ObserveAPI(endpoint, time.Since(start).Seconds())
	if err != nil {
		a.audit(mode, false, callee, "open failed")
		return fmt.Errorf("%w: %v", ErrLaunchFailed, err)
	}
	a.audit(mode, true, callee, "")
	return nil
}

// allow shares its bucket across video + audio so alternating modes cannot
// evade the cap.
func (a *Adapter) allow() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.now()
	cutoff := now.Add(-rateWindow)
	kept := a.bucket[:0]
	for _, t := range a.bucket {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	a.bucket = kept
	if len(a.bucket) >= rateMax {
		return false
	}
	a.bucket = append(a.bucket, now)
	return true
}

func (a *Adapter) audit(mode string, success bool, callee, reason string) {
	sum := sha256.Sum256([]byte(callee))
	a.sink.Audit(AuditRow{
		Kind:       "facetime_initiate",
		Timestamp:  a.now().UTC().Format(time.RFC3339),
		Mode:       mode,
		Success:    success,
		CalleeHash: hex.EncodeToString(sum[:])[:8],
		Reason:     reason,
	})
}
