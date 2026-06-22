package higgsfield

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func hfServer(t *testing.T, status int, body string, gotAuth, gotPath *string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if gotAuth != nil {
			*gotAuth = r.Header.Get("Authorization")
		}
		if gotPath != nil {
			*gotPath = r.URL.Path
		}
		if status != 0 {
			w.WriteHeader(status)
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestHTTPTransport_HTTPStatus_MapsSentinel(t *testing.T) {
	t.Parallel()
	cases := []struct {
		status int
		want   error
	}{
		{http.StatusUnauthorized, ErrAuthFailed},
		{http.StatusForbidden, ErrAuthFailed},
		{http.StatusTooManyRequests, ErrRateLimited},
	}
	for _, tc := range cases {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			srv := hfServer(t, tc.status, "", nil, nil)
			tr := NewHTTPTransport(srv.Client(), srv.URL)
			if _, _, err := tr.GenerateImage(context.Background(), "k", "p"); !errors.Is(err, tc.want) {
				t.Fatalf("status %d -> %v, want %v", tc.status, err, tc.want)
			}
		})
	}
}

func TestHTTPTransport_5xx_ServerError(t *testing.T) {
	t.Parallel()
	srv := hfServer(t, http.StatusInternalServerError, "boom", nil, nil)
	tr := NewHTTPTransport(srv.Client(), srv.URL)
	_, _, err := tr.GenerateImage(context.Background(), "k", "p")
	if err == nil || errors.Is(err, ErrAuthFailed) || errors.Is(err, ErrRateLimited) {
		t.Fatalf("5xx must surface a non-sentinel error: %v", err)
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("error should carry HTTP status: %v", err)
	}
}

// Content-policy refusal comes back as HTTP 200 with a typed body code; the
// transport must classify it as ErrPolicyRefused, NOT silent success.
func TestHTTPTransport_ContentPolicy_MapsPolicyRefused(t *testing.T) {
	t.Parallel()
	body := `{"error":{"code":"content_policy_violation","message":"refused"}}`
	srv := hfServer(t, http.StatusOK, body, nil, nil)
	tr := NewHTTPTransport(srv.Client(), srv.URL)
	if _, _, err := tr.GenerateImage(context.Background(), "k", "p"); !errors.Is(err, ErrPolicyRefused) {
		t.Fatalf("err = %v, want ErrPolicyRefused", err)
	}
}

func TestHTTPTransport_GenerateImage_SetsBearer(t *testing.T) {
	t.Parallel()
	var gotAuth, gotPath string
	body := `{"image_base64":"iVBORw0KGgo=","mime":"image/png"}`
	srv := hfServer(t, http.StatusOK, body, &gotAuth, &gotPath)
	tr := NewHTTPTransport(srv.Client(), srv.URL)

	b, mime, err := tr.GenerateImage(context.Background(), "hf-key", "p")
	if err != nil {
		t.Fatalf("GenerateImage: %v", err)
	}
	if mime != "image/png" || len(b) == 0 {
		t.Fatalf("mime=%q len=%d", mime, len(b))
	}
	if gotAuth != "Bearer hf-key" {
		t.Fatalf("Authorization = %q, want Bearer hf-key", gotAuth)
	}
	if !strings.Contains(gotPath, "image") {
		t.Fatalf("path = %q, want image endpoint", gotPath)
	}
}

func TestHTTPTransport_SubmitClip_ReturnsJobID(t *testing.T) {
	t.Parallel()
	body := `{"job_id":"job-42"}`
	srv := hfServer(t, http.StatusOK, body, nil, nil)
	tr := NewHTTPTransport(srv.Client(), srv.URL)

	id, err := tr.SubmitClip(context.Background(), "k", "p", 8)
	if err != nil {
		t.Fatalf("SubmitClip: %v", err)
	}
	if id != "job-42" {
		t.Fatalf("jobID = %q, want job-42", id)
	}
}

func TestHTTPTransport_PollClip_ReturnsStatusAndMP4(t *testing.T) {
	t.Parallel()
	body := `{"status":"ready","mp4_base64":"AAAA"}`
	srv := hfServer(t, http.StatusOK, body, nil, nil)
	tr := NewHTTPTransport(srv.Client(), srv.URL)

	status, mp4, err := tr.PollClip(context.Background(), "k", "job-42")
	if err != nil {
		t.Fatalf("PollClip: %v", err)
	}
	if status != "ready" {
		t.Fatalf("status = %q, want ready", status)
	}
	if len(mp4) == 0 {
		t.Fatal("expected non-empty mp4")
	}
}

func TestHTTPTransport_PollClip_PendingStatus_NoMP4(t *testing.T) {
	t.Parallel()
	body := `{"status":"pending"}`
	srv := hfServer(t, http.StatusOK, body, nil, nil)
	tr := NewHTTPTransport(srv.Client(), srv.URL)

	status, mp4, err := tr.PollClip(context.Background(), "k", "job-42")
	if err != nil {
		t.Fatalf("PollClip pending: %v", err)
	}
	if status != "pending" {
		t.Fatalf("status = %q, want pending", status)
	}
	if len(mp4) != 0 {
		t.Fatalf("pending must not return mp4 bytes: %d", len(mp4))
	}
}

func TestNewHTTPTransport_DefaultsBaseURL(t *testing.T) {
	t.Parallel()
	tr := NewHTTPTransport(nil, "")
	if tr.base != apiBase {
		t.Fatalf("base = %q, want %q", tr.base, apiBase)
	}
	if tr.hc == nil {
		t.Fatal("nil HTTP client not defaulted")
	}
}

func TestClassifyError_KnownCodes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		code string
		want error
	}{
		{"content_policy_violation", ErrPolicyRefused},
		{"safety_refused", ErrPolicyRefused},
		{"invalid_api_key", ErrAuthFailed},
		{"rate_limited", ErrRateLimited},
	}
	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			if err := classifyError(tc.code); !errors.Is(err, tc.want) {
				t.Fatalf("code %q -> %v, want %v", tc.code, err, tc.want)
			}
		})
	}
}

func TestClassifyError_UnknownCode_NotSilent(t *testing.T) {
	t.Parallel()
	err := classifyError("something_weird")
	if err == nil {
		t.Fatal("unknown code must still be non-nil")
	}
	if errors.Is(err, ErrPolicyRefused) || errors.Is(err, ErrAuthFailed) || errors.Is(err, ErrRateLimited) {
		t.Fatalf("unknown code aliased to sentinel: %v", err)
	}
	if !strings.Contains(err.Error(), "something_weird") {
		t.Fatalf("error should carry the code: %v", err)
	}
}
