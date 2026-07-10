package connect

import (
	"context"
	"errors"
	"fmt"
	"os/exec"

	"golang.org/x/oauth2"
)

var ErrNativeUnavailable = errors.New("connect: macOS native binary unavailable")

// nativeMacProvider connects a macOS-native adapter: consent is the OS Automation/FDA grant, the written token is a presence marker, never sent as auth.
type nativeMacProvider struct {
	name      string
	binary    string
	tokenPath string
	lookPath  func(string) (string, error)
}

func newNativeMac(name, binary string) *nativeMacProvider {
	return &nativeMacProvider{
		name:      name,
		binary:    binary,
		tokenPath: DefaultTokenPath(name),
		lookPath:  exec.LookPath,
	}
}

func (p *nativeMacProvider) Name() string      { return p.name }
func (p *nativeMacProvider) Scopes() []string  { return []string{"connect:" + p.name} }
func (p *nativeMacProvider) TokenPath() string { return p.tokenPath }

func (p *nativeMacProvider) authorize(_ context.Context, _ PromptFn) (*oauth2.Token, error) {
	if _, err := p.lookPath(p.binary); err != nil {
		return nil, fmt.Errorf("%w: %s (%v)", ErrNativeUnavailable, p.binary, err)
	}
	return &oauth2.Token{AccessToken: "connected"}, nil
}
