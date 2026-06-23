package slack

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const apiBase = "https://slack.com/api"

// HTTPTransport is the default Transport: direct form-encoded Slack Web API calls over net/http.
type HTTPTransport struct {
	hc   *http.Client
	base string
}

func NewHTTPTransport(hc *http.Client, base string) *HTTPTransport {
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	if base == "" {
		base = apiBase
	}
	return &HTTPTransport{hc: hc, base: strings.TrimRight(base, "/")}
}

func (h *HTTPTransport) ListChannels(ctx context.Context, bot string) ([]Channel, error) {
	var resp struct {
		Channels []Channel `json:"channels"`
	}
	if err := h.call(ctx, bot, "conversations.list", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Channels, nil
}

func (h *HTTPTransport) PostMessage(ctx context.Context, bot, channel, text string) error {
	form := url.Values{"channel": {channel}, "text": {text}}
	return h.call(ctx, bot, "chat.postMessage", form, nil)
}

func (h *HTTPTransport) GetThread(ctx context.Context, bot, channel, ts string) (Thread, error) {
	var resp struct {
		Messages []Message `json:"messages"`
	}
	form := url.Values{"channel": {channel}, "ts": {ts}}
	if err := h.call(ctx, bot, "conversations.replies", form, &resp); err != nil {
		return Thread{}, err
	}
	return Thread{Channel: channel, TS: ts, Replies: resp.Messages}, nil
}

func (h *HTTPTransport) Search(ctx context.Context, user, query string) ([]Result, error) {
	var resp struct {
		Messages struct {
			Matches []struct {
				Channel   struct{ ID string } `json:"channel"`
				TS        string              `json:"ts"`
				Text      string              `json:"text"`
				Permalink string              `json:"permalink"`
			} `json:"matches"`
		} `json:"messages"`
	}
	form := url.Values{"query": {query}}
	if err := h.call(ctx, user, "search.messages", form, &resp); err != nil {
		return nil, err
	}
	out := make([]Result, 0, len(resp.Messages.Matches))
	for _, m := range resp.Messages.Matches {
		out = append(out, Result{Channel: m.Channel.ID, TS: m.TS, Snippet: m.Text, URL: m.Permalink})
	}
	return out, nil
}

// call: ok field — not HTTP status — is Slack's authoritative success signal (200+ok:false is a failure).
func (h *HTTPTransport) call(ctx context.Context, bearer, method string, form url.Values, out any) error {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.base+"/"+method, body)
	if err != nil {
		return fmt.Errorf("slack: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := h.hc.Do(req)
	if err != nil {
		return fmt.Errorf("slack: http: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return ErrAuthFailed
	case http.StatusTooManyRequests:
		return ErrRateLimited
	}
	if resp.StatusCode >= 500 {
		return fmt.Errorf("slack: server error %d", resp.StatusCode)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("slack: read body: %w", err)
	}
	var env struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("slack: decode: %w", err)
	}
	if !env.OK {
		return classifySlackError(env.Error)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("slack: decode: %w", err)
	}
	return nil
}

func classifySlackError(code string) error {
	switch code {
	case "invalid_auth", "not_authed", "token_revoked", "account_inactive", "no_permission":
		return fmt.Errorf("%w: %s", ErrAuthFailed, code)
	case "channel_not_found", "thread_not_found", "message_not_found":
		return fmt.Errorf("%w: %s", ErrNotFound, code)
	case "ratelimited", "rate_limited":
		return fmt.Errorf("%w: %s", ErrRateLimited, code)
	case "is_archived", "msg_too_long", "no_text", "invalid_channel":
		return fmt.Errorf("%w: %s", ErrInvalidChannel, code)
	default:
		return fmt.Errorf("slack: api error: %s", code)
	}
}
