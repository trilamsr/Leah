package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/trilam/leah/internal/contracts"
	commsout "github.com/trilam/leah/internal/comms/out"
)

// writeDiscordToken drops a connect-shaped token file so connected() passes.
func writeDiscordToken(t *testing.T, sd string) {
	t.Helper()
	p := filepath.Join(sd, "secrets", "discord-token.json")
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(`{"access_token":"bot-tok"}`), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestBriefNotifierAppendsDiscordWhenConnectedAndChannelSet(t *testing.T) {
	sd := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", sd)
	t.Setenv("LEAH_PUSHOVER_USER", "")
	t.Setenv("LEAH_PUSHOVER_TOKEN", "")
	t.Setenv("LEAH_BRIEF_WHATSAPP_TO", "")
	writeDiscordToken(t, sd)
	t.Setenv("LEAH_BRIEF_DISCORD_CHANNEL", "C123")

	f := buildBriefNotifier(connectedDiscordAdapter())
	if got := len(f.Notifiers); got != 3 {
		t.Errorf("desktop+voice+discord = 3, got %d", got)
	}
}

func TestBriefNotifierOmitsDiscordWhenChannelUnset(t *testing.T) {
	sd := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", sd)
	t.Setenv("LEAH_PUSHOVER_USER", "")
	t.Setenv("LEAH_PUSHOVER_TOKEN", "")
	t.Setenv("LEAH_BRIEF_WHATSAPP_TO", "")
	writeDiscordToken(t, sd)
	t.Setenv("LEAH_BRIEF_DISCORD_CHANNEL", "")

	f := buildBriefNotifier(connectedDiscordAdapter())
	if got := len(f.Notifiers); got != 2 {
		t.Errorf("discord channel unset → desktop+voice = 2, got %d", got)
	}
}

func TestBriefNotifierOmitsDiscordWhenNotConnected(t *testing.T) {
	sd := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", sd)
	t.Setenv("LEAH_PUSHOVER_USER", "")
	t.Setenv("LEAH_PUSHOVER_TOKEN", "")
	t.Setenv("LEAH_BRIEF_WHATSAPP_TO", "")
	t.Setenv("LEAH_BRIEF_DISCORD_CHANNEL", "C123")

	f := buildBriefNotifier(connectedDiscordAdapter())
	if got := len(f.Notifiers); got != 2 {
		t.Errorf("discord token absent → desktop+voice = 2, got %d", got)
	}
}

func TestSpamStatsFor_ReadsLiveLimiter(t *testing.T) {
	sd := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", sd)
	writeDiscordToken(t, sd)
	a := connectedDiscordAdapter()
	if a == nil {
		t.Fatal("adapter must be connected")
	}
	provider := spamStatsFor(a)
	if provider == nil {
		t.Fatal("provider must be non-nil when connected")
	}
	rows := provider()
	if len(rows) != 1 || rows[0].Adapter != "discord" {
		t.Fatalf("rows: got %+v, want one discord row", rows)
	}
}

func TestSpamStatsFor_NilWhenUnconnected(t *testing.T) {
	if spamStatsFor(nil) != nil {
		t.Error("provider must be nil when adapter is nil")
	}
}

// failing remote notifier MUST NOT block the others (Fanout isolation).
func TestBriefNotifierFanoutIsolatesRemoteFailure(t *testing.T) {
	desktop := &countingNotifier{}
	remote := &countingNotifier{err: errors.New("remote down")}
	f := &commsout.Fanout{Notifiers: []contracts.Notifier{desktop, remote}}

	if err := f.Notify(context.Background(), "T", "B"); err == nil {
		t.Error("want joined error from failing remote")
	}
	if desktop.calls != 1 {
		t.Errorf("desktop must still fire despite remote failure, calls=%d", desktop.calls)
	}
}

type countingNotifier struct {
	calls int
	err   error
}

func (c *countingNotifier) Notify(context.Context, string, string) error {
	c.calls++
	return c.err
}
