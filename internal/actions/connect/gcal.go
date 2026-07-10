package connect

import (
	"context"
	"time"

	"golang.org/x/oauth2"
)

// calendar.events covers list + create — read-only would force a second
// scope on CreateEvent, so wider scope keeps consent single-shot.
var gcalScopes = []string{"https://www.googleapis.com/auth/calendar.events"}

type gcalProvider struct {
	clientID     string
	clientSecret string
	tokenPath    string
	deviceURL    string
	tokenURL     string
	pollInterval time.Duration
}

func NewGcal(clientID, clientSecret string) Provider {
	return &gcalProvider{
		clientID:     clientID,
		clientSecret: clientSecret,
		tokenPath:    DefaultTokenPath("gcal"),
		deviceURL:    googleDeviceURL,
		tokenURL:     googleTokenURL,
		pollInterval: pollDelayFromEnv(),
	}
}

func (p *gcalProvider) Name() string      { return "gcal" }
func (p *gcalProvider) Scopes() []string  { return gcalScopes }
func (p *gcalProvider) TokenPath() string { return p.tokenPath }

func (p *gcalProvider) authorize(ctx context.Context, prompt PromptFn) (*oauth2.Token, error) {
	return runDeviceCodeFlow(ctx, deviceCodeConfig{
		clientID:     p.clientID,
		clientSecret: p.clientSecret,
		scopes:       gcalScopes,
		deviceURL:    p.deviceURL,
		tokenURL:     p.tokenURL,
		minPoll:      p.pollInterval,
	}, prompt)
}
