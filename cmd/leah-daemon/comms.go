package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"

	"github.com/trilam/leah/internal/adapters/discord"
	"github.com/trilam/leah/internal/adapters/whatsapp"
	"github.com/trilam/leah/internal/connect"
	"github.com/trilam/leah/internal/notify"
	"github.com/trilam/leah/internal/voice"
	"github.com/trilam/leah/internal/web"
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

// newDiscordNotifier wraps the shared adapter as a brief notifier; nil unless a
// channel is set and the adapter is connected. Sharing the adapter means the
// dashboard spam panel reports the same limiter that gates these sends.
func newDiscordNotifier(a *discord.Adapter) *notify.DiscordNotify {
	channel := os.Getenv("LEAH_BRIEF_DISCORD_CHANNEL")
	if channel == "" || a == nil {
		return nil
	}
	n := &notify.DiscordNotify{Poster: a, ChannelID: channel}
	if os.Getenv("LEAH_BRIEF_VOICE_REMOTE") == "1" {
		if s, ok := voice.NewTTS().(voice.Synthesizer); ok {
			n.Synth = s
		}
	}
	return n
}

// newWhatsAppNotifier returns nil unless whatsapp is connected, phone-id set, and a recipient set.
func newWhatsAppNotifier() *notify.WhatsAppNotify {
	to := os.Getenv("LEAH_BRIEF_WHATSAPP_TO")
	phoneID := os.Getenv("LEAH_WHATSAPP_PHONE_ID")
	if to == "" || phoneID == "" {
		return nil
	}
	path := connect.DefaultTokenPath("whatsapp")
	if !connected(path) {
		return nil
	}
	a, err := whatsapp.New(whatsapp.Config{
		Attestor:           noopAttestor{},
		TokenSource:        waToken{token: fileToken{path}, phoneID: phoneID},
		HTTP:               http.DefaultClient,
		RecipientAllowlist: []string{to},
	})
	if err != nil {
		return nil
	}
	return &notify.WhatsAppNotify{Sender: a, To: to}
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

// waToken satisfies the whatsapp 4-method TokenSource; SendText uses only the first two.
type waToken struct {
	token   fileToken
	phoneID string
}

func (w waToken) Token(ctx context.Context) (string, error)     { return w.token.Token(ctx) }
func (w waToken) PhoneNumberID(context.Context) (string, error) { return w.phoneID, nil }
func (w waToken) VerifyToken(context.Context) (string, error) {
	return os.Getenv("LEAH_WHATSAPP_VERIFY_TOKEN"), nil
}
func (w waToken) AppSecret(context.Context) (string, error) {
	return os.Getenv("LEAH_WHATSAPP_APP_SECRET"), nil
}
