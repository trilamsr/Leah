package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/trilam/leah/internal/actions/adapters/discord"
	"github.com/trilam/leah/internal/platform/attest"
	"github.com/trilam/leah/internal/platform/audit"
	commsin "github.com/trilam/leah/internal/input/commsin"
	"github.com/trilam/leah/internal/thinking/recommend"
	"github.com/trilam/leah/internal/platform/testutil"
)

// fakeDialer satisfies discord.WebSocketDialer with a frame-replaying conn so
// the daemon-wiring test exercises Subscribe → handler → router without a
// real gateway.
type fakeDialer struct {
	frames [][]byte
	dialed atomic.Int32
}

func (d *fakeDialer) Dial(_ context.Context, _, _ string) (discord.WebSocketConn, error) {
	d.dialed.Add(1)
	return &fakeConn{frames: d.frames, done: make(chan struct{})}, nil
}

type fakeConn struct {
	frames   [][]byte
	i        int
	closed   atomic.Bool
	done     chan struct{}
	doneOnce sync.Once
}

func (c *fakeConn) ReadMessage() ([]byte, error) {
	if c.closed.Load() {
		return nil, context.Canceled
	}
	if c.i >= len(c.frames) {
		// Real gateway never sends EOF synchronously after the last frame —
		// block until Close so the dispatch goroutine drains without spinning.
		<-c.done
		return nil, context.Canceled
	}
	f := c.frames[c.i]
	c.i++
	return f, nil
}

func (c *fakeConn) Close() error {
	c.closed.Store(true)
	c.doneOnce.Do(func() { close(c.done) })
	return nil
}

func writeTokenFile(t *testing.T, sd, integration, token string) string {
	t.Helper()
	dir := filepath.Join(sd, "connect")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, integration+".json")
	body, _ := json.Marshal(struct {
		AccessToken string `json:"access_token"`
	}{AccessToken: token})
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

// TestStartInboundDiscordDisabledByDefault: env unset → no dialer constructed,
// no error, no goroutine. Default-OFF is the load-bearing safety property
// (spec §6 — token leak guard in test runs).
func TestStartInboundDiscordDisabledByDefault(t *testing.T) {
	sd := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", sd)
	t.Setenv("LEAH_INBOUND_DISCORD", "")

	dialer := &fakeDialer{}
	a := &audit.Logger{Path: filepath.Join(sd, "audit.jsonl")}
	eng := recommend.NewMemoryEngine(a)

	stop, err := startInboundDiscord(context.Background(), inboundOpts{
		StateDir: sd,
		Engine:   eng,
		Audit:    a,
		Dialer:   dialer,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stop == nil {
		t.Fatalf("stop must be non-nil (caller defers it unconditionally)")
	}
	stop()
	if dialer.dialed.Load() != 0 {
		t.Errorf("env-gate violated: dialer dialed %d times with env unset", dialer.dialed.Load())
	}
}

// TestStartInboundDiscordSilentWhenUnconnected: env on but no discord token →
// silent absence, no error (matches the F2 notifier connected-and-configured
// pattern).
func TestStartInboundDiscordSilentWhenUnconnected(t *testing.T) {
	sd := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", sd)
	t.Setenv("LEAH_INBOUND_DISCORD", "1")
	t.Setenv("LEAH_INBOUND_DISCORD_CHANNELS", "C1,C2")

	dialer := &fakeDialer{}
	a := &audit.Logger{Path: filepath.Join(sd, "audit.jsonl")}
	eng := recommend.NewMemoryEngine(a)

	stop, err := startInboundDiscord(context.Background(), inboundOpts{
		StateDir: sd,
		Engine:   eng,
		Audit:    a,
		Dialer:   dialer,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	stop()
	if dialer.dialed.Load() != 0 {
		t.Errorf("unconnected → dialer dialed %d times (must be 0)", dialer.dialed.Load())
	}
}

// TestStartInboundDiscordSilentWhenNoChannels: env on + token, but allowlist
// empty → silent skip. A subscription with no channels would still attest the
// subscribe scope — pointless burn.
func TestStartInboundDiscordSilentWhenNoChannels(t *testing.T) {
	sd := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", sd)
	t.Setenv("LEAH_INBOUND_DISCORD", "1")
	t.Setenv("LEAH_INBOUND_DISCORD_CHANNELS", "")
	tokenPath := writeTokenFile(t, sd, "discord", "T")
	t.Setenv("LEAH_INBOUND_DISCORD_TOKEN_PATH", tokenPath)

	dialer := &fakeDialer{}
	a := &audit.Logger{Path: filepath.Join(sd, "audit.jsonl")}
	eng := recommend.NewMemoryEngine(a)

	stop, err := startInboundDiscord(context.Background(), inboundOpts{
		StateDir: sd,
		Engine:   eng,
		Audit:    a,
		Dialer:   dialer,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	stop()
	if dialer.dialed.Load() != 0 {
		t.Errorf("empty allowlist → dialer must not dial; got %d", dialer.dialed.Load())
	}
}

// TestStartInboundDiscordWiresRouter: env on + token + channels + fake frame
// carrying a known pending reply → router dispatches to engine. Proves the
// Subscribe → handler → Router.Handle path is live.
func TestStartInboundDiscordWiresRouter(t *testing.T) {
	sd := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", sd)
	t.Setenv("LEAH_INBOUND_DISCORD", "1")
	t.Setenv("LEAH_INBOUND_DISCORD_CHANNELS", "C1")
	t.Setenv("LEAH_INBOUND_DISCORD_GUILDS", "G1")
	tokenPath := writeTokenFile(t, sd, "discord", "T")
	t.Setenv("LEAH_INBOUND_DISCORD_TOKEN_PATH", tokenPath)

	frame, _ := json.Marshal(map[string]any{
		"op": 0,
		"t":  "MESSAGE_CREATE",
		"d": map[string]any{
			"channel_id": "C1",
			"guild_id":   "G1",
			"content":    "yes",
			"author":     map[string]string{"id": "U1"},
			"timestamp":  time.Now().Format(time.RFC3339Nano),
		},
	})
	dialer := &fakeDialer{frames: [][]byte{frame}}

	a := &audit.Logger{Path: filepath.Join(sd, "audit.jsonl")}
	eng := recommend.NewMemoryEngine(a)

	// Pre-stage the pending rec + enrollment so the reply clears both
	// binding layers and exercises the full Accept→Apply path.
	rec := recommend.Recommendation{ID: "rec1", Pattern: "p", Tier: recommend.TierSilent, Source: "test"}
	eng.Seed(rec)

	pending := commsin.NewMemoryPendingStore()
	if err := pending.Put(commsin.Pending{RecID: "rec1", Channel: "discord", ConvID: "C1", PeerID: "U1", SentAt: time.Now()}); err != nil {
		t.Fatalf("put pending: %v", err)
	}
	enroll := commsin.NewMemoryEnrollStore()
	_ = enroll.Enroll("discord", "U1")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	att := &recordingAttestor{}
	stop, err := startInboundDiscord(ctx, inboundOpts{
		StateDir: sd,
		Engine:   eng,
		Audit:    a,
		Dialer:   dialer,
		Pending:  pending,
		Enroll:   enroll,
		Attestor: att,
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer stop()

	// Single-use Take firing on the staged pending entry proves the full
	// dialer → read → dispatch → router → engine wiring.
	testutil.Eventually(t, 2*time.Second, 10*time.Millisecond, func() bool {
		if dialer.dialed.Load() == 0 {
			return false
		}
		_, ok := pending.Peek("discord", "C1")
		return !ok
	})
}

// recordingAttestor captures the scopes presented to it so tests can assert
// the load-bearing rule (spec §4.2): a self-build rec accepted from a remote
// channel MUST attest the self-build scope, not the weaker inbound-apply.
type recordingAttestor struct {
	mu     sync.Mutex
	scopes []string
}

func (r *recordingAttestor) Attest(_ context.Context, scope string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.scopes = append(r.scopes, scope)
	return nil
}

func (r *recordingAttestor) Scopes() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.scopes))
	copy(out, r.scopes)
	return out
}

// TestStartInboundDiscordSelfBuildAttestsSelfBuildScope is the spec §7
// load-bearing test: a self-build rec accepted via Discord attests
// `self-build`, NOT `inbound-apply`. If this regresses, a compromised Discord
// account could trick the operator into approving a self-build merge with the
// weaker inbound-apply prompt — exactly the spec §8 privilege-escalation risk
// the consent contract exists to prevent.
func TestStartInboundDiscordSelfBuildAttestsSelfBuildScope(t *testing.T) {
	sd := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", sd)
	t.Setenv("LEAH_INBOUND_DISCORD", "1")
	t.Setenv("LEAH_INBOUND_DISCORD_CHANNELS", "C1")
	t.Setenv("LEAH_INBOUND_DISCORD_GUILDS", "G1")
	tokenPath := writeTokenFile(t, sd, "discord", "T")
	t.Setenv("LEAH_INBOUND_DISCORD_TOKEN_PATH", tokenPath)

	frame, _ := json.Marshal(map[string]any{
		"op": 0, "t": "MESSAGE_CREATE",
		"d": map[string]any{
			"channel_id": "C1", "guild_id": "G1", "content": "approve",
			"author":    map[string]string{"id": "U1"},
			"timestamp": time.Now().Format(time.RFC3339Nano),
		},
	})
	dialer := &fakeDialer{frames: [][]byte{frame}}

	a := &audit.Logger{Path: filepath.Join(sd, "audit.jsonl")}
	eng := recommend.NewMemoryEngine(a)
	// TierConfirm + Source "self-build" → resolver MUST return self-build scope.
	eng.Seed(recommend.Recommendation{
		ID: "rec-sb", Pattern: "self-build:merge-pr", Source: "self-build.dispatcher",
		Tier: recommend.TierConfirm,
	})
	pending := commsin.NewMemoryPendingStore()
	_ = pending.Put(commsin.Pending{RecID: "rec-sb", Channel: "discord", ConvID: "C1", PeerID: "U1", SentAt: time.Now()})
	enroll := commsin.NewMemoryEnrollStore()
	_ = enroll.Enroll("discord", "U1")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	att := &recordingAttestor{}
	stop, err := startInboundDiscord(ctx, inboundOpts{
		StateDir: sd, Engine: eng, Audit: a, Dialer: dialer,
		Pending: pending, Enroll: enroll, Attestor: att,
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer stop()

	testutil.Eventually(t, 2*time.Second, 10*time.Millisecond, func() bool {
		return len(att.Scopes()) > 0
	})
	if got := att.Scopes()[0]; got != attest.ScopeSelfBuild {
		t.Fatalf("scope downgrade: got %q want %q (spec §4.2 — remote origin must NOT downgrade gate)", got, attest.ScopeSelfBuild)
	}
}

// TestStartInboundDiscordDefaultsFailClosed: with no attestor injected, the
// default failClosedAttestor must deny accepts. Proves spec §4.2 layer-2
// can't be bypassed by an unwired daemon (better deny-on-accept than silently
// apply via a noop).
func TestStartInboundDiscordDefaultsFailClosed(t *testing.T) {
	g := &commsin.Gate{
		Attestor: failClosedAttestor{},
		Resolver: commsin.StaticScopeResolver{Scope: attest.ScopeInboundApply},
		Store:    commsin.NewMemoryEnrollStore(),
	}
	err := g.Authorize(context.Background(),
		commsin.Pending{RecID: "x", Channel: "discord", ConvID: "C1", PeerID: "U1"},
		commsin.IntentAccept)
	if err == nil {
		t.Fatalf("default attestor must fail closed on Authorize; got nil error")
	}
}
