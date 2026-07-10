package connect

import (
	"context"
	"time"

	"golang.org/x/oauth2"
)

const (
	googleDeviceURL = "https://oauth2.googleapis.com/device/code"
	googleTokenURL  = "https://oauth2.googleapis.com/token"
)

// gmail.modify covers list + mark + send — one consent prompt unlocks the
// whole adapter surface; finer scoping would force a re-consent on Send.
var gmailScopes = []string{"https://www.googleapis.com/auth/gmail.modify"}

type gmailProvider struct {
	clientID     string
	clientSecret string
	tokenPath    string
	deviceURL    string
	tokenURL     string
	pollInterval time.Duration
}

func NewGmail(clientID, clientSecret string) Provider {
	return &gmailProvider{
		clientID:     clientID,
		clientSecret: clientSecret,
		tokenPath:    DefaultTokenPath("gmail"),
		deviceURL:    googleDeviceURL,
		tokenURL:     googleTokenURL,
		pollInterval: pollDelayFromEnv(),
	}
}

func (p *gmailProvider) Name() string      { return "gmail" }
func (p *gmailProvider) Scopes() []string  { return gmailScopes }
func (p *gmailProvider) TokenPath() string { return p.tokenPath }

func (p *gmailProvider) authorize(ctx context.Context, prompt PromptFn) (*oauth2.Token, error) {
	return runDeviceCodeFlow(ctx, deviceCodeConfig{
		clientID:     p.clientID,
		clientSecret: p.clientSecret,
		scopes:       gmailScopes,
		deviceURL:    p.deviceURL,
		tokenURL:     p.tokenURL,
		minPoll:      p.pollInterval,
	}, prompt)
}
