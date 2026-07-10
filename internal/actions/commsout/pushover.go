package commsout

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/trilam/leah/internal/platform/keychain"
)

const defaultPushoverAPI = "https://api.pushover.net/1/messages.json"

// Pushover sends a push to the operator's phone via the Pushover REST API.
// Credentials are sourced from LEAH_PUSHOVER_USER + LEAH_PUSHOVER_TOKEN; if
// either is unset, Notify returns an error and the caller falls back to the
// desktop banner only.
type Pushover struct {
	User   string
	Token  string
	HTTP   *http.Client
	APIURL string
}

// NewPushover returns a Pushover wired to env + Keychain (env wins) with the
// default API URL. HTTP client has explicit 10s timeout — daemon error path
// can't block on slow DNS / Pushover endpoint. Keychain read errors are
// swallowed so a locked Keychain still allows env-only credentials.
func NewPushover() *Pushover {
	user, _ := keychain.LoadPushoverUser()
	token, _ := keychain.LoadPushoverToken()
	return &Pushover{
		User:   user,
		Token:  token,
		HTTP:   &http.Client{Timeout: 10 * time.Second},
		APIURL: defaultPushoverAPI,
	}
}

// Notify posts title + body to Pushover. Returns a credentials error
// (without hitting the network) when User or Token is empty so the daemon
// can degrade gracefully to desktop-only push.
func (p *Pushover) Notify(ctx context.Context, title, body string) error {
	if p.User == "" || p.Token == "" {
		return fmt.Errorf("pushover credentials not set (LEAH_PUSHOVER_USER/TOKEN)")
	}
	form := url.Values{}
	form.Set("user", p.User)
	form.Set("token", p.Token)
	form.Set("title", title)
	form.Set("message", body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.APIURL, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("new pushover request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("pushover: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		errBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("pushover status %d: %s", resp.StatusCode, errBody)
	}
	var r struct {
		Status int `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return fmt.Errorf("decode pushover resp: %w", err)
	}
	if r.Status != 1 {
		return fmt.Errorf("pushover api status=%d", r.Status)
	}
	return nil
}
