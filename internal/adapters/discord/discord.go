package discord

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/trilam/leah/internal/contracts"
)

const (
	ScopePost         = "discord:post"
	ScopeListChannels = "discord:list_channels"
)

const (
	maxOutboundPerWindow = 10
	rateWindow           = 60 * time.Second
)

const defaultBaseURL = "https://discord.com/api/v10"

var (
	ErrAttestationDenied = errors.New("discord: attestation denied")
	ErrAttestorRequired  = errors.New("discord: Config.Attestor required (operator-attestation gate)")
	ErrTokenSrcRequired  = errors.New("discord: Config.TokenSource required")
	ErrGuildNotAllowed   = errors.New("discord: guild not on allowlist")
	ErrRateLimited       = errors.New("discord: rate limit exceeded")
	ErrPostFailed        = errors.New("discord: post failed")
	ErrEmptyChannel      = errors.New("discord: empty channelID")
	ErrEmptyBody         = errors.New("discord: empty body")
)

type Channel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// AuditRow omits plaintext channel/guild ids by design (spec §7).
type AuditRow struct {
	Kind        string
	Success     bool
	ChannelHash string
	GuildHash   string
	BodyLen     int
	Reason      string
}

type AuditSink interface {
	Record(row AuditRow)
}

type Attestor = contracts.Attestor

type TokenSource = contracts.TokenSource

type Config struct {
	Attestor       Attestor
	TokenSource    TokenSource
	HTTPClient     contracts.HTTPClient
	BaseURL        string
	GuildAllowlist []string
	Audit          AuditSink
	Now            func() time.Time
}

type Adapter struct {
	att            Attestor
	ts             TokenSource
	http           contracts.HTTPClient
	baseURL        string
	guildAllowlist []string
	audit          AuditSink
	now            func() time.Time

	mu    sync.Mutex
	sends map[string][]time.Time
}

func New(cfg Config) (*Adapter, error) {
	if cfg.Attestor == nil {
		return nil, ErrAttestorRequired
	}
	if cfg.TokenSource == nil {
		return nil, ErrTokenSrcRequired
	}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = http.DefaultClient
	}
	base := cfg.BaseURL
	if base == "" {
		base = defaultBaseURL
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Adapter{
		att:            cfg.Attestor,
		ts:             cfg.TokenSource,
		http:           hc,
		baseURL:        base,
		guildAllowlist: cfg.GuildAllowlist,
		audit:          cfg.Audit,
		now:            now,
		sends:          map[string][]time.Time{},
	}, nil
}

// PostMessage order is load-bearing: validate -> rate-limit -> attest -> token -> POST.
// Token-load BEFORE attestation would leak the bot token via panic traces on deny.
func (a *Adapter) PostMessage(ctx context.Context, channelID, body string) error {
	hash := shortHash(channelID)
	bodyLen := utf8.RuneCountInString(body)

	if channelID == "" {
		return ErrEmptyChannel
	}
	if body == "" {
		return ErrEmptyBody
	}
	if !a.allow(channelID) {
		a.record(AuditRow{Kind: "discord_post", ChannelHash: hash, BodyLen: bodyLen, Reason: "rate_limited"})
		return ErrRateLimited
	}
	if err := a.att.Attest(ctx, ScopePost); err != nil {
		a.record(AuditRow{Kind: "discord_post", ChannelHash: hash, BodyLen: bodyLen, Reason: "attestation_denied"})
		return fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	tok, err := a.ts.Token(ctx)
	if err != nil {
		a.record(AuditRow{Kind: "discord_post", ChannelHash: hash, BodyLen: bodyLen, Reason: "token_load_failed"})
		return fmt.Errorf("discord: token load: %w", err)
	}

	payload, _ := json.Marshal(struct {
		Content string `json:"content"`
	}{Content: body})
	url := fmt.Sprintf("%s/channels/%s/messages", a.baseURL, channelID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		a.record(AuditRow{Kind: "discord_post", ChannelHash: hash, BodyLen: bodyLen, Reason: "request_build"})
		return fmt.Errorf("%w: %v", ErrPostFailed, err)
	}
	req.Header.Set("Authorization", "Bot "+tok)
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.http.Do(req)
	if err != nil {
		a.record(AuditRow{Kind: "discord_post", ChannelHash: hash, BodyLen: bodyLen, Reason: "http_error"})
		return fmt.Errorf("%w: %v", ErrPostFailed, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		a.record(AuditRow{Kind: "discord_post", ChannelHash: hash, BodyLen: bodyLen, Reason: fmt.Sprintf("status_%d", resp.StatusCode)})
		return fmt.Errorf("%w: status %d", ErrPostFailed, resp.StatusCode)
	}
	a.record(AuditRow{Kind: "discord_post", Success: true, ChannelHash: hash, BodyLen: bodyLen})
	return nil
}

// ListChannels enumerates a guild's channels. Allowlist check runs BEFORE
// attestation so a misrouted guild never burns an operator prompt.
func (a *Adapter) ListChannels(ctx context.Context, guildID string) ([]Channel, error) {
	gHash := shortHash(guildID)
	if guildID == "" {
		return nil, ErrGuildNotAllowed
	}
	if !a.guildAllowed(guildID) {
		a.record(AuditRow{Kind: "discord_list_channels", GuildHash: gHash, Reason: "guild_not_allowed"})
		return nil, ErrGuildNotAllowed
	}
	if err := a.att.Attest(ctx, ScopeListChannels); err != nil {
		a.record(AuditRow{Kind: "discord_list_channels", GuildHash: gHash, Reason: "attestation_denied"})
		return nil, fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	tok, err := a.ts.Token(ctx)
	if err != nil {
		return nil, fmt.Errorf("discord: token load: %w", err)
	}

	url := fmt.Sprintf("%s/guilds/%s/channels", a.baseURL, guildID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bot "+tok)
	resp, err := a.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("discord: list channels status %d", resp.StatusCode)
	}

	// Discord returns type as int; map the MVP three (text=0, voice=2, thread=11)
	// onto stable string tags so callers do not depend on Discord's numbering.
	var raw []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Type int    `json:"type"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	out := make([]Channel, 0, len(raw))
	for _, r := range raw {
		out = append(out, Channel{ID: r.ID, Name: r.Name, Type: channelTypeName(r.Type)})
	}
	a.record(AuditRow{Kind: "discord_list_channels", Success: true, GuildHash: gHash})
	return out, nil
}

func (a *Adapter) guildAllowed(guildID string) bool {
	for _, g := range a.guildAllowlist {
		if g == guildID {
			return true
		}
	}
	return false
}

func (a *Adapter) allow(channelID string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	cutoff := a.now().Add(-rateWindow)
	kept := a.sends[channelID][:0]
	for _, t := range a.sends[channelID] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	a.sends[channelID] = kept
	if len(a.sends[channelID]) >= maxOutboundPerWindow {
		return false
	}
	a.sends[channelID] = append(a.sends[channelID], a.now())
	return true
}

func (a *Adapter) record(r AuditRow) {
	if a.audit == nil {
		return
	}
	a.audit.Record(r)
}

func shortHash(s string) string {
	if s == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:8]
}

func channelTypeName(t int) string {
	switch t {
	case 0, 5:
		return "text"
	case 1, 3:
		return "dm"
	case 10, 11, 12:
		return "thread"
	default:
		return "other"
	}
}
