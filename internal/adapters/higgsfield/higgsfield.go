package higgsfield

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/trilam/leah/internal/obs/connectadapter"
)

var (
	ErrAttestationDenied = errors.New("higgsfield: attestation denied")
	ErrAuthFailed        = errors.New("higgsfield: auth failed")
	ErrRateLimited       = errors.New("higgsfield: rate limited")
	ErrPolicyRefused     = errors.New("higgsfield: content policy refused")
	ErrInvalidPrompt     = errors.New("higgsfield: invalid prompt")
)

const (
	ScopeGenerateImage = "higgsfield:generate_image"
	ScopeSubmitClip    = "higgsfield:submit_clip"
	ScopePollClip      = "higgsfield:poll_clip"
)

// Clip-length bounds match the strategist spec: 5-10s short-form social clips.
const (
	minClipSeconds = 5
	maxClipSeconds = 10
)

type Attestor interface {
	Attest(ctx context.Context, scope string) error
}

type TokenSource interface {
	Token(ctx context.Context) (string, error)
}

type Transport interface {
	GenerateImage(ctx context.Context, key, prompt string) (bytes []byte, mime string, err error)
	SubmitClip(ctx context.Context, key, prompt string, seconds int) (jobID string, err error)
	PollClip(ctx context.Context, key, jobID string) (status string, mp4 []byte, err error)
}

// AuditRow omits plaintext prompt bytes by design: PromptHash is sha256-hex
// truncated to 8, matching the slack adapter's ChannelHash convention.
type AuditRow struct {
	Kind       string
	Success    bool
	PromptHash string
	Reason     string
}

type AuditSink interface {
	Record(row AuditRow)
}

type Config struct {
	Attestor    Attestor
	TokenSource TokenSource
	Transport   Transport
	Audit       AuditSink
	Now         func() time.Time
	Metrics     *connectadapter.Metrics
}

type Adapter struct {
	att   Attestor
	ts    TokenSource
	tr    Transport
	audit AuditSink
	now   func() time.Time
	m     *connectadapter.Metrics
}

func New(cfg Config) (*Adapter, error) {
	if cfg.Attestor == nil {
		return nil, errors.New("higgsfield: Config.Attestor required (operator-attestation gate)")
	}
	if cfg.TokenSource == nil {
		return nil, errors.New("higgsfield: Config.TokenSource required")
	}
	if cfg.Transport == nil {
		return nil, errors.New("higgsfield: Config.Transport required")
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Adapter{
		att:   cfg.Attestor,
		ts:    cfg.TokenSource,
		tr:    cfg.Transport,
		audit: cfg.Audit,
		now:   now,
		m:     cfg.Metrics,
	}, nil
}

// Order: validate -> attest -> token -> transport -> audit. Validation MUST
// precede attestation so a typo never consumes an operator-consent prompt.
func (a *Adapter) GenerateImage(ctx context.Context, prompt string) ([]byte, string, error) {
	hash := promptHash(prompt)
	if prompt == "" {
		a.record(AuditRow{Kind: "higgsfield_generate_image", PromptHash: hash, Reason: "invalid"})
		return nil, "", ErrInvalidPrompt
	}
	if err := a.att.Attest(ctx, ScopeGenerateImage); err != nil {
		a.record(AuditRow{Kind: "higgsfield_generate_image", PromptHash: hash, Reason: "attestation_denied"})
		return nil, "", fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	key, err := a.ts.Token(ctx)
	if err != nil {
		a.record(AuditRow{Kind: "higgsfield_generate_image", PromptHash: hash, Reason: "token_load"})
		return nil, "", fmt.Errorf("higgsfield: token: %w", err)
	}
	start := time.Now()
	b, mime, err := a.tr.GenerateImage(ctx, key, prompt)
	a.m.ObserveAPI("generate_image", time.Since(start).Seconds())
	if err != nil {
		a.record(AuditRow{Kind: "higgsfield_generate_image", PromptHash: hash, Reason: err.Error()})
		return nil, "", err
	}
	a.record(AuditRow{Kind: "higgsfield_generate_image", Success: true, PromptHash: hash})
	return b, mime, nil
}

func (a *Adapter) SubmitClip(ctx context.Context, prompt string, seconds int) (string, error) {
	hash := promptHash(prompt)
	if prompt == "" || seconds < minClipSeconds || seconds > maxClipSeconds {
		a.record(AuditRow{Kind: "higgsfield_submit_clip", PromptHash: hash, Reason: "invalid"})
		return "", ErrInvalidPrompt
	}
	if err := a.att.Attest(ctx, ScopeSubmitClip); err != nil {
		a.record(AuditRow{Kind: "higgsfield_submit_clip", PromptHash: hash, Reason: "attestation_denied"})
		return "", fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	key, err := a.ts.Token(ctx)
	if err != nil {
		a.record(AuditRow{Kind: "higgsfield_submit_clip", PromptHash: hash, Reason: "token_load"})
		return "", fmt.Errorf("higgsfield: token: %w", err)
	}
	start := time.Now()
	id, err := a.tr.SubmitClip(ctx, key, prompt, seconds)
	a.m.ObserveAPI("submit_clip", time.Since(start).Seconds())
	if err != nil {
		a.record(AuditRow{Kind: "higgsfield_submit_clip", PromptHash: hash, Reason: err.Error()})
		return "", err
	}
	a.record(AuditRow{Kind: "higgsfield_submit_clip", Success: true, PromptHash: hash})
	return id, nil
}

// PollClip hashes jobID into PromptHash for symmetry with the other audit
// rows — the job opaque is operator-correlatable but the bytes never leak.
func (a *Adapter) PollClip(ctx context.Context, jobID string) (string, []byte, error) {
	hash := promptHash(jobID)
	if jobID == "" {
		a.record(AuditRow{Kind: "higgsfield_poll_clip", PromptHash: hash, Reason: "invalid"})
		return "", nil, ErrInvalidPrompt
	}
	if err := a.att.Attest(ctx, ScopePollClip); err != nil {
		a.record(AuditRow{Kind: "higgsfield_poll_clip", PromptHash: hash, Reason: "attestation_denied"})
		return "", nil, fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	key, err := a.ts.Token(ctx)
	if err != nil {
		a.record(AuditRow{Kind: "higgsfield_poll_clip", PromptHash: hash, Reason: "token_load"})
		return "", nil, fmt.Errorf("higgsfield: token: %w", err)
	}
	start := time.Now()
	status, mp4, err := a.tr.PollClip(ctx, key, jobID)
	a.m.ObserveAPI("poll_clip", time.Since(start).Seconds())
	if err != nil {
		a.record(AuditRow{Kind: "higgsfield_poll_clip", PromptHash: hash, Reason: err.Error()})
		return "", nil, err
	}
	a.record(AuditRow{Kind: "higgsfield_poll_clip", Success: true, PromptHash: hash})
	return status, mp4, nil
}

func (a *Adapter) record(r AuditRow) {
	if a.audit == nil {
		return
	}
	a.audit.Record(r)
}

func promptHash(p string) string {
	sum := sha256.Sum256([]byte(p))
	return hex.EncodeToString(sum[:])[:8]
}
