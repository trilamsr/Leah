package slack

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/trilam/leah/internal/contracts"
	"github.com/trilam/leah/internal/obs/connectadapter"
)

var (
	ErrAttestationDenied = errors.New("slack: attestation denied")
	ErrAuthFailed        = errors.New("slack: auth failed")
	ErrNotFound          = errors.New("slack: not found")
	ErrRateLimited       = errors.New("slack: rate limited")
	ErrInvalidChannel    = errors.New("slack: invalid channel")
)

const (
	ScopeListChannels = "slack:list_channels"
	ScopePost         = "slack:post_message"
	ScopeGetThread    = "slack:get_thread"
	ScopeSearch       = "slack:search"
)

// Per-channel rate limit. Matches Slack's published 1-msg-per-channel-per-sec
// guidance — avoids 429 retry storms in the dispatcher burst path.
const postWindow = time.Second

type Channel struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	IsIM    bool   `json:"is_im"`
	IsGroup bool   `json:"is_group"`
}

type Message struct {
	User string `json:"user"`
	TS   string `json:"ts"`
	Text string `json:"text"`
}

type Thread struct {
	Channel string
	TS      string
	Replies []Message
}

type Result struct {
	Channel string
	TS      string
	Snippet string
	URL     string
}

// TokenSource yields per-token-class bearers. Search needs UserToken (Bot
// Tokens cannot search); everything else uses BotToken. Empty UserToken is a
// declined-User-Token signal, not an error.
type TokenSource interface {
	BotToken(ctx context.Context) (string, error)
	UserToken(ctx context.Context) (string, error)
}

// Transport is the seam to slack-go/slack (or a raw HTTP client). Keeping it
// an interface defers the SDK require until a combined tidy.
type Transport interface {
	ListChannels(ctx context.Context, bot string) ([]Channel, error)
	PostMessage(ctx context.Context, bot, channel, text string) error
	GetThread(ctx context.Context, bot, channel, ts string) (Thread, error)
	Search(ctx context.Context, user, query string) ([]Result, error)
}

// AuditRow omits plaintext channel / text bytes by design (spec §7).
type AuditRow struct {
	Kind        string
	Success     bool
	ChannelHash string
	TextLen     int
	Reason      string
}

type AuditSink interface {
	Record(row AuditRow)
}

type Config struct {
	Attestor    contracts.Attestor
	TokenSource TokenSource
	Transport   Transport
	Audit       AuditSink
	Now         func() time.Time
	// Metrics is optional — nil is a no-op (connectadapter contract), so
	// existing callers keep working without a registry.
	Metrics *connectadapter.Metrics
}

type Adapter struct {
	att   contracts.Attestor
	ts    TokenSource
	tr    Transport
	audit AuditSink
	now   func() time.Time
	m     *connectadapter.Metrics

	mu       sync.Mutex
	lastPost map[string]time.Time
}

func New(cfg Config) (*Adapter, error) {
	if cfg.Attestor == nil {
		return nil, errors.New("slack: Config.Attestor required (operator-attestation gate)")
	}
	if cfg.TokenSource == nil {
		return nil, errors.New("slack: Config.TokenSource required")
	}
	if cfg.Transport == nil {
		return nil, errors.New("slack: Config.Transport required")
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Adapter{
		att:      cfg.Attestor,
		ts:       cfg.TokenSource,
		tr:       cfg.Transport,
		audit:    cfg.Audit,
		now:      now,
		m:        cfg.Metrics,
		lastPost: make(map[string]time.Time),
	}, nil
}

func (a *Adapter) ListChannels(ctx context.Context) ([]Channel, error) {
	if err := a.att.Attest(ctx, ScopeListChannels); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	tok, err := a.ts.BotToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("slack: bot token: %w", err)
	}
	start := time.Now()
	chs, err := a.tr.ListChannels(ctx, tok)
	a.m.ObserveAPI("list_channels", time.Since(start).Seconds())
	return chs, err
}

// Order: validate -> rate-limit -> attest -> token -> transport -> audit.
// Validation and rate-limit MUST precede attestation so a typo or burst does
// not consume an operator-consent prompt.
func (a *Adapter) PostMessage(ctx context.Context, channel, text string) error {
	hash := channelHash(channel)
	textLen := utf8.RuneCountInString(text)
	if channel == "" || text == "" {
		a.record(AuditRow{Kind: "slack_post_message", ChannelHash: hash, TextLen: textLen, Reason: "invalid"})
		return ErrInvalidChannel
	}
	if !a.allowPost(channel) {
		a.record(AuditRow{Kind: "slack_post_message", ChannelHash: hash, TextLen: textLen, Reason: "rate_limited"})
		return ErrRateLimited
	}
	if err := a.att.Attest(ctx, ScopePost); err != nil {
		a.record(AuditRow{Kind: "slack_post_message", ChannelHash: hash, TextLen: textLen, Reason: "attestation_denied"})
		return fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	tok, err := a.ts.BotToken(ctx)
	if err != nil {
		a.record(AuditRow{Kind: "slack_post_message", ChannelHash: hash, TextLen: textLen, Reason: "token_load"})
		return fmt.Errorf("slack: bot token: %w", err)
	}
	start := time.Now()
	err = a.tr.PostMessage(ctx, tok, channel, text)
	a.m.ObserveAPI("post_message", time.Since(start).Seconds())
	if err != nil {
		a.record(AuditRow{Kind: "slack_post_message", ChannelHash: hash, TextLen: textLen, Reason: err.Error()})
		return err
	}
	a.record(AuditRow{Kind: "slack_post_message", Success: true, ChannelHash: hash, TextLen: textLen})
	return nil
}

func (a *Adapter) GetThread(ctx context.Context, channel, ts string) (Thread, error) {
	if channel == "" || ts == "" {
		return Thread{}, ErrInvalidChannel
	}
	if err := a.att.Attest(ctx, ScopeGetThread); err != nil {
		return Thread{}, fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	tok, err := a.ts.BotToken(ctx)
	if err != nil {
		return Thread{}, fmt.Errorf("slack: bot token: %w", err)
	}
	start := time.Now()
	th, err := a.tr.GetThread(ctx, tok, channel, ts)
	a.m.ObserveAPI("get_thread", time.Since(start).Seconds())
	return th, err
}

// Search is the only RPC that reaches for the User Token. Empty UserToken
// short-circuits to ErrAuthFailed BEFORE attestation — no consent prompt for
// a call that cannot succeed.
func (a *Adapter) Search(ctx context.Context, query string) ([]Result, error) {
	if query == "" {
		return nil, ErrInvalidChannel
	}
	user, err := a.ts.UserToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("slack: user token: %w", err)
	}
	if user == "" {
		return nil, ErrAuthFailed
	}
	if err := a.att.Attest(ctx, ScopeSearch); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	start := time.Now()
	res, err := a.tr.Search(ctx, user, query)
	a.m.ObserveAPI("search", time.Since(start).Seconds())
	return res, err
}

func (a *Adapter) allowPost(channel string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.now()
	if last, ok := a.lastPost[channel]; ok && now.Sub(last) < postWindow {
		return false
	}
	a.lastPost[channel] = now
	return true
}

func (a *Adapter) record(r AuditRow) {
	if a.audit == nil {
		return
	}
	a.audit.Record(r)
}

func channelHash(c string) string {
	sum := sha256.Sum256([]byte(c))
	return hex.EncodeToString(sum[:])[:8]
}
