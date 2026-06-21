package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/trilam/leah/internal/adapters/discord"
	"github.com/trilam/leah/internal/attestation"
	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/connect"
	"github.com/trilam/leah/internal/contracts"
	"github.com/trilam/leah/internal/inbound"
	"github.com/trilam/leah/internal/recommend"
)

const (
	envInboundDiscordEnable    = "LEAH_INBOUND_DISCORD"
	envInboundDiscordChannels  = "LEAH_INBOUND_DISCORD_CHANNELS"
	envInboundDiscordGuilds    = "LEAH_INBOUND_DISCORD_GUILDS"
	envInboundDiscordTokenPath = "LEAH_INBOUND_DISCORD_TOKEN_PATH"
)

// inboundOpts collects daemon-side seams. Pending/Enroll/Dialer/Attestor are
// optional; production fills Pending+Enroll with persistent defaults, leaves
// Dialer nil (no production gateway dialer wired in v1 — spec §6 step 4), and
// defaults Attestor to a fail-closed shim so an opt-in deployment without a
// real interactive attestor errors on Accept rather than silently mutating
// (spec §4.2: per-action consent collected locally, never noop).
type inboundOpts struct {
	StateDir string
	Engine   *recommend.MemoryEngine
	Audit    *audit.Logger
	Dialer   discord.WebSocketDialer
	Pending  inbound.PendingStore
	Enroll   inbound.EnrollStore
	Attestor contracts.Attestor
}

// startInboundDiscord wires the F3 inbound-reply router behind
// LEAH_INBOUND_DISCORD=1. Default OFF — no env, no subscribe, no token read,
// no goroutine. Silent absence on missing token or empty channel allowlist
// matches the F2 notifier connected-and-configured contract: the operator
// fixes the missing piece without a daemon crash.
//
// Returns a stop func the caller defers unconditionally (nil-stop on the
// silent-absence paths so the caller stays branch-free).
func startInboundDiscord(ctx context.Context, opts inboundOpts) (func(), error) {
	noop := func() {}
	if os.Getenv(envInboundDiscordEnable) != "1" {
		return noop, nil
	}

	channels := parseCSV(os.Getenv(envInboundDiscordChannels))
	if len(channels) == 0 {
		return noop, nil
	}

	tokenPath := os.Getenv(envInboundDiscordTokenPath)
	if tokenPath == "" {
		tokenPath = connect.DefaultTokenPath("discord")
	}
	if !connected(tokenPath) {
		return noop, nil
	}

	guilds := parseCSV(os.Getenv(envInboundDiscordGuilds))

	adpt, err := discord.New(discord.Config{
		Attestor:        noopAttestor{},
		TokenSource:     fileToken{path: tokenPath},
		GuildAllowlist:  guilds,
		WebSocketDialer: opts.Dialer,
	})
	if err != nil {
		return noop, fmt.Errorf("inbound discord adapter: %w", err)
	}

	pending := opts.Pending
	if pending == nil {
		pending = inbound.NewMemoryPendingStore()
	}
	enroll := opts.Enroll
	if enroll == nil {
		// Persistent enroll across restarts: the layer-1 trust grant is the
		// load-bearing one and re-prompting on every boot would train
		// dismissal. Falls back to memory if the file open fails so the
		// daemon still starts.
		path := filepath.Join(opts.StateDir, "inbound-enroll.json")
		if fs, ferr := inbound.OpenFileEnrollStore(path); ferr == nil {
			enroll = fs
		} else {
			enroll = inbound.NewMemoryEnrollStore()
		}
	}

	att := opts.Attestor
	if att == nil {
		att = failClosedAttestor{}
	}

	router := &inbound.Router{
		Pending: pending,
		Consent: &inbound.Gate{
			Attestor: att,
			Resolver: recScopeResolver{eng: opts.Engine},
			Store:    enroll,
		},
		Classify: inbound.NewRegexClassifier(),
		Engine:   engineByID{eng: opts.Engine},
		Audit:    auditSink(opts.Audit),
	}

	subCtx, cancel := context.WithCancel(ctx)
	handler := func(m discord.Message) {
		_ = router.Handle(subCtx, discordToReply(m))
	}

	if err := adpt.Subscribe(subCtx, channels, handler); err != nil {
		cancel()
		// A dial/attestation/missing-dialer failure on opt-in is loud — the
		// operator set the flag and expects the subscription to come up;
		// silent skip here would hide a misconfiguration.
		return noop, fmt.Errorf("inbound discord subscribe: %w", err)
	}
	return cancel, nil
}

func parseCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := parts[:0]
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// discordToReply normalizes a Discord MESSAGE_CREATE into the transport-
// agnostic Reply. Voice bytes are carried but ignored downstream until the
// STT inbound path lands (spec §6 — Voice deferred).
func discordToReply(m discord.Message) inbound.Reply {
	return inbound.Reply{
		Channel:  "discord",
		PeerID:   m.AuthorID,
		ConvID:   m.ChannelID,
		Text:     m.Body,
		Voice:    m.Voice,
		Received: m.Timestamp,
	}
}

// engineByID adapts MemoryEngine to the router's id-keyed Engine. Apply
// resolves the id back to the seeded Recommendation via Propose() so the
// router stays decoupled from internal/recommend's struct shape.
type engineByID struct{ eng *recommend.MemoryEngine }

func (e engineByID) Accept(ctx context.Context, id string) error {
	return e.eng.Accept(ctx, id)
}
func (e engineByID) Reject(ctx context.Context, id string) error {
	return e.eng.Reject(ctx, id)
}
func (e engineByID) Apply(ctx context.Context, id string) error {
	recs, err := e.eng.Propose(ctx)
	if err != nil {
		return err
	}
	for _, r := range recs {
		if r.ID == id {
			return e.eng.Apply(ctx, r)
		}
	}
	return errors.New("inbound: rec not found for apply: " + id)
}

// recScopeResolver maps a pending RecID back to the rec via the engine and
// returns the scope keyed to the rec's blast radius (spec §4.2). The
// load-bearing rule: a self-build rec accepted from a remote channel attests
// the *self-build* scope, NOT the weaker inbound-apply — remote origin never
// downgrades the gate. A missing rec returns inbound-apply as a conservative
// default rather than "" (no-scope = no prompt), so a race where Take fires
// between Authorize and engine lookup still gates rather than silently
// applies. Source/Pattern-prefix matching keeps the daemon decoupled from a
// (not-yet-shipped) per-rec scope field.
type recScopeResolver struct{ eng *recommend.MemoryEngine }

func (r recScopeResolver) ScopeFor(recID string) string {
	if r.eng == nil {
		return attestation.ScopeInboundApply
	}
	recs, err := r.eng.Propose(context.Background())
	if err != nil {
		return attestation.ScopeInboundApply
	}
	for _, rec := range recs {
		if rec.ID != recID {
			continue
		}
		switch {
		case hasPrefix(rec.Source, "self-build"), hasPrefix(rec.Pattern, "self-build"):
			return attestation.ScopeSelfBuild
		case hasPrefix(rec.Source, "self-upgrade"), hasPrefix(rec.Pattern, "self-upgrade"):
			return attestation.ScopeSelfUpgrade
		default:
			return attestation.ScopeInboundApply
		}
	}
	return attestation.ScopeInboundApply
}

func hasPrefix(s, prefix string) bool { return strings.HasPrefix(s, prefix) }

// failClosedAttestor is the daemon's default when no interactive attestor is
// wired. Always denies — accepts from remote will error rather than silently
// apply (spec §4.2: per-action consent must be collected locally; noop on a
// non-attestor would bypass the load-bearing layer-2 gate). The operator
// wires a real attestor when the daemon's interactive surface lands.
type failClosedAttestor struct{}

func (failClosedAttestor) Attest(context.Context, string) error {
	return errors.New("inbound: no interactive attestor wired; remote accept denied (spec §4.2)")
}

func auditSink(a *audit.Logger) func(inbound.AuditRow) {
	if a == nil {
		return nil
	}
	return func(row inbound.AuditRow) {
		_ = a.Append(audit.Entry{
			Kind:    "inbound_reply",
			Outcome: row.Outcome,
			Detail:  row.Channel + ":" + row.RecID,
		})
	}
}
