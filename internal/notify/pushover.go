package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

const defaultPushoverAPI = "https://api.pushover.net/1/messages.json"

type Pushover struct {
	User   string
	Token  string
	HTTP   *http.Client
	APIURL string
}

func NewPushover() *Pushover {
	return &Pushover{
		User:   os.Getenv("LEAH_PUSHOVER_USER"),
		Token:  os.Getenv("LEAH_PUSHOVER_TOKEN"),
		HTTP:   http.DefaultClient,
		APIURL: defaultPushoverAPI,
	}
}

func (p *Pushover) Notify(ctx context.Context, title, body string) error {
	if p.User == "" || p.Token == "" {
		return fmt.Errorf("pushover credentials not set (LEAH_PUSHOVER_USER/TOKEN)")
	}
	form := url.Values{}
	form.Set("user", p.User)
	form.Set("token", p.Token)
	form.Set("title", title)
	form.Set("message", body)

	req, err := http.NewRequestWithContext(ctx, "POST", p.APIURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("pushover: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("pushover status %d: %s", resp.StatusCode, body)
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
