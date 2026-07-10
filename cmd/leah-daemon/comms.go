package main

import (
	"context"
	"encoding/json"
	"os"

	"github.com/trilam/leah/internal/actions/adapters/discord"
	"github.com/trilam/leah/internal/actions/connect"
	"github.com/trilam/leah/internal/platform/web"
)

// connectedDiscordAdapter builds the adapter when discord is connected, else nil.
func connectedDiscordAdapter() *discord.Adapter {
	path := connect.DefaultTokenPath("discord")
	if !connected(path) {
		return nil
	}
	a, err := discord.New(discord.Config{
		Attestor:    noopAttestor{},
		TokenSource: fileToken{path},
	})
	if err != nil {
		return nil
	}
	return a
}

// spamStatsFor returns the dashboard SpamStats provider reading the shared
// adapter's live limiter, or nil when not connected (panel renders empty).
func spamStatsFor(a *discord.Adapter) func() []web.SpamStat {
	if a == nil {
		return nil
	}
	return func() []web.SpamStat {
		st := a.LimiterStats()
		return []web.SpamStat{{Adapter: "discord", Sends: st.Sends, Denied: st.Denied}}
	}
}

// fileToken re-reads the connect-written bearer per call so rotation needs no restart.
type fileToken struct{ path string }

func (f fileToken) Token(context.Context) (string, error) {
	buf, err := os.ReadFile(f.path)
	if err != nil {
		return "", err
	}
	var t struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(buf, &t); err != nil {
		return "", err
	}
	return t.AccessToken, nil
}
