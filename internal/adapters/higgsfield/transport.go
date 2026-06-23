package higgsfield

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// apiBase is a placeholder — Higgsfield's real path is operator-discovered at
// wire-up time. NewHTTPTransport's `base` arg lets the strategist config
// override it without a code change.
const apiBase = "https://higgsfield.ai/api/v1"

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

type errEnvelope struct {
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (h *HTTPTransport) GenerateImage(ctx context.Context, key, prompt string) ([]byte, string, error) {
	var resp struct {
		errEnvelope
		ImageBase64 string `json:"image_base64"`
		MIME        string `json:"mime"`
	}
	if err := h.call(ctx, key, "/image/generate", map[string]any{"prompt": prompt}, &resp); err != nil {
		return nil, "", err
	}
	if resp.Error != nil {
		return nil, "", classifyError(resp.Error.Code)
	}
	raw, err := base64.StdEncoding.DecodeString(resp.ImageBase64)
	if err != nil {
		return nil, "", fmt.Errorf("higgsfield: decode image: %w", err)
	}
	return raw, resp.MIME, nil
}

func (h *HTTPTransport) SubmitClip(ctx context.Context, key, prompt string, seconds int) (string, error) {
	var resp struct {
		errEnvelope
		JobID string `json:"job_id"`
	}
	body := map[string]any{"prompt": prompt, "seconds": seconds}
	if err := h.call(ctx, key, "/clip/submit", body, &resp); err != nil {
		return "", err
	}
	if resp.Error != nil {
		return "", classifyError(resp.Error.Code)
	}
	return resp.JobID, nil
}

func (h *HTTPTransport) PollClip(ctx context.Context, key, jobID string) (string, []byte, error) {
	var resp struct {
		errEnvelope
		Status    string `json:"status"`
		MP4Base64 string `json:"mp4_base64"`
	}
	if err := h.call(ctx, key, "/clip/poll", map[string]any{"job_id": jobID}, &resp); err != nil {
		return "", nil, err
	}
	if resp.Error != nil {
		return "", nil, classifyError(resp.Error.Code)
	}
	if resp.MP4Base64 == "" {
		return resp.Status, nil, nil
	}
	mp4, err := base64.StdEncoding.DecodeString(resp.MP4Base64)
	if err != nil {
		return "", nil, fmt.Errorf("higgsfield: decode mp4: %w", err)
	}
	return resp.Status, mp4, nil
}

func (h *HTTPTransport) call(ctx context.Context, bearer, path string, payload any, out any) error {
	buf, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("higgsfield: encode: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.base+path, bytes.NewReader(buf))
	if err != nil {
		return fmt.Errorf("higgsfield: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.hc.Do(req)
	if err != nil {
		return fmt.Errorf("higgsfield: http: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return ErrAuthFailed
	case http.StatusTooManyRequests:
		return ErrRateLimited
	}
	if resp.StatusCode >= 500 {
		return fmt.Errorf("higgsfield: server error %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("higgsfield: read body: %w", err)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("higgsfield: decode: %w", err)
	}
	return nil
}

func classifyError(code string) error {
	switch code {
	case "content_policy_violation", "safety_refused", "policy_refused":
		return fmt.Errorf("%w: %s", ErrPolicyRefused, code)
	case "invalid_api_key", "unauthorized", "forbidden":
		return fmt.Errorf("%w: %s", ErrAuthFailed, code)
	case "rate_limited", "too_many_requests":
		return fmt.Errorf("%w: %s", ErrRateLimited, code)
	default:
		return fmt.Errorf("higgsfield: api error: %s", code)
	}
}
