package connect

import (
	"context"
	"time"

	"golang.org/x/oauth2"
)

// GitHub device-code flow is RFC 8628; endpoint shapes are documented at
// https://docs.github.com/en/apps/oauth-apps/building-oauth-apps/authorizing-oauth-apps#device-flow.
// repo + read:user is the minimum surface for issue/PR + identity probes;
// finer scoping forces a re-consent dance every time an adapter call grows.
const (
	githubDeviceURL = "https://github.com/login/device/code"
	githubTokenURL  = "https://github.com/login/oauth/access_token"
)

var githubScopes = []string{"repo", "read:user"}

type githubProvider struct {
	clientID     string
	clientSecret string
	tokenPath    string
	deviceURL    string
	tokenURL     string
	pollInterval time.Duration
}

func NewGitHub(clientID, clientSecret string) Provider {
	return &githubProvider{
		clientID:     clientID,
		clientSecret: clientSecret,
		tokenPath:    DefaultTokenPath("github"),
		deviceURL:    githubDeviceURL,
		tokenURL:     githubTokenURL,
		pollInterval: pollDelayFromEnv(),
	}
}

func (p *githubProvider) Name() string      { return "github" }
func (p *githubProvider) Scopes() []string  { return githubScopes }
func (p *githubProvider) TokenPath() string { return p.tokenPath }

func (p *githubProvider) authorize(ctx context.Context, prompt PromptFn) (*oauth2.Token, error) {
	return runDeviceCodeFlow(ctx, deviceCodeConfig{
		clientID:     p.clientID,
		clientSecret: p.clientSecret,
		scopes:       githubScopes,
		deviceURL:    p.deviceURL,
		tokenURL:     p.tokenURL,
		minPoll:      p.pollInterval,
	}, prompt)
}
