// Package connect is the first-launch integration auth boundary.
// See docs/engineer/briefs/2026-06-09-first-launch-connect.md.
package connect

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"github.com/trilam/leah/internal/contracts"
)

var (
	ErrAttestationDenied = errors.New("connect: attestation denied")
	ErrDeviceAuth        = errors.New("connect: device authorization failed")
	ErrTokenExchange     = errors.New("connect: token exchange failed")
	ErrUnknownProvider   = errors.New("connect: unknown provider")
)

// oauthHTTPClient bounds OAuth device-code + poll round trips so a hung IdP
// can't park the connect wizard indefinitely.
var oauthHTTPClient = &http.Client{Timeout: 10 * time.Second}

type PromptFn func(verificationURL, userCode string)

type Provider interface {
	Name() string
	Scopes() []string
	TokenPath() string
	authorize(ctx context.Context, prompt PromptFn) (*oauth2.Token, error)
}

type Status struct {
	Name       string
	Authorized bool
	TokenPath  string
}

// Authorize: attestation FIRST, then device-code exchange, then on-disk
// write. Denied attestation MUST short-circuit before the network call.
func Authorize(ctx context.Context, p Provider, att contracts.Attestor, prompt PromptFn) (*oauth2.Token, error) {
	if err := att.Attest(ctx, "connect:"+p.Name()); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	tok, err := p.authorize(ctx, prompt)
	if err != nil {
		return nil, err
	}
	if err := WriteToken(p.TokenPath(), tok); err != nil {
		return nil, err
	}
	return tok, nil
}

// WriteToken serializes tok at path with 0600. Atomic write via tmp+rename
// closes the race where a pre-existing world-readable file at `path` keeps
// its prior mode between WriteFile and Chmod (round 9 keychain audit).
func WriteToken(path string, tok *oauth2.Token) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("connect: mkdir token dir: %w", err)
	}
	buf, err := json.MarshalIndent(tok, "", "  ")
	if err != nil {
		return fmt.Errorf("connect: marshal token: %w", err)
	}
	// O_CREATE|O_EXCL+0o600 forces a fresh inode with the right mode from
	// the start; rename() then atomically swaps it over any prior file.
	tmp := path + ".tmp"
	_ = os.Remove(tmp)
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("connect: open token tmp: %w", err)
	}
	if _, err := f.Write(buf); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("connect: write token: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("connect: close token tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("connect: rename token: %w", err)
	}
	return nil
}

type deviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURL string `json:"verification_url"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	Error        string `json:"error"`
}

func runDeviceCodeFlow(ctx context.Context, cfg deviceCodeConfig, prompt PromptFn) (*oauth2.Token, error) {
	dev, err := requestDeviceCode(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if prompt != nil {
		prompt(dev.VerificationURL, dev.UserCode)
	}
	interval := time.Duration(dev.Interval) * time.Second
	if interval <= 0 {
		interval = cfg.minPoll
	}
	deadline := time.Now().Add(time.Duration(dev.ExpiresIn) * time.Second)
	for {
		tok, retry, err := pollToken(ctx, cfg, dev.DeviceCode)
		if err != nil {
			return nil, err
		}
		if !retry {
			return tok, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("%w: device code expired", ErrTokenExchange)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}
	}
}

type deviceCodeConfig struct {
	clientID     string
	clientSecret string
	scopes       []string
	deviceURL    string
	tokenURL     string
	minPoll      time.Duration
}

func requestDeviceCode(ctx context.Context, cfg deviceCodeConfig) (*deviceCodeResponse, error) {
	form := url.Values{
		"client_id": {cfg.clientID},
		"scope":     {strings.Join(cfg.scopes, " ")},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.deviceURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("%w: build request: %v", ErrDeviceAuth, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := oauthHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDeviceAuth, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: HTTP %d", ErrDeviceAuth, resp.StatusCode)
	}
	var out deviceCodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("%w: decode: %v", ErrDeviceAuth, err)
	}
	return &out, nil
}

// pollToken returns (tok, retry, err). retry=true means the operator has not
// approved yet — caller waits `interval` then retries.
func pollToken(ctx context.Context, cfg deviceCodeConfig, deviceCode string) (*oauth2.Token, bool, error) {
	form := url.Values{
		"client_id":     {cfg.clientID},
		"client_secret": {cfg.clientSecret},
		"device_code":   {deviceCode},
		"grant_type":    {"urn:ietf:params:oauth:grant-type:device_code"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, false, fmt.Errorf("%w: build request: %v", ErrTokenExchange, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := oauthHTTPClient.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("%w: %v", ErrTokenExchange, err)
	}
	defer func() { _ = resp.Body.Close() }()
	var body tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, false, fmt.Errorf("%w: decode: %v", ErrTokenExchange, err)
	}
	if body.Error == "authorization_pending" || body.Error == "slow_down" {
		return nil, true, nil
	}
	if body.Error != "" {
		return nil, false, fmt.Errorf("%w: %s", ErrTokenExchange, body.Error)
	}
	if body.AccessToken == "" {
		return nil, false, fmt.Errorf("%w: empty access_token", ErrTokenExchange)
	}
	tok := &oauth2.Token{
		AccessToken:  body.AccessToken,
		TokenType:    body.TokenType,
		RefreshToken: body.RefreshToken,
	}
	if body.ExpiresIn > 0 {
		tok.Expiry = time.Now().Add(time.Duration(body.ExpiresIn) * time.Second)
	}
	return tok, false, nil
}

func DefaultTokenPath(name string) string {
	d := os.Getenv("LEAH_STATE_DIR")
	if d == "" {
		home, _ := os.UserHomeDir()
		d = filepath.Join(home, ".leah-state")
	}
	return filepath.Join(d, "secrets", name+"-token.json")
}

func pollDelayFromEnv() time.Duration {
	if v := os.Getenv("LEAH_CONNECT_POLL_MS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return time.Duration(n) * time.Millisecond
		}
	}
	return 5 * time.Second
}
