package atlassian

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func TestBasicAuthHeader(t *testing.T) {
	t.Parallel()
	got := BasicAuthHeader("ops@acme.com", "tok123")
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("ops@acme.com:tok123"))
	if got != want {
		t.Fatalf("BasicAuthHeader = %q, want %q", got, want)
	}
	if strings.Contains(got, "tok123") {
		t.Fatalf("raw token leaked into header: %q", got)
	}
}

func TestMapStatus(t *testing.T) {
	t.Parallel()
	cases := []struct {
		code int
		want error
	}{
		{200, nil},
		{201, nil},
		{299, nil},
		{401, ErrAuthFailed},
		{403, ErrAuthFailed},
		{404, ErrNotFound},
		{429, ErrRateLimited},
		{500, ErrUpstream},
	}
	for _, tc := range cases {
		if err := MapStatus(tc.code); !errors.Is(err, tc.want) {
			t.Fatalf("MapStatus(%d) = %v, want %v", tc.code, err, tc.want)
		}
	}
}

func TestVerifyWorkspace(t *testing.T) {
	t.Parallel()
	if err := VerifyWorkspace("acme", "acme"); err != nil {
		t.Fatalf("matching workspace rejected: %v", err)
	}
	if err := VerifyWorkspace("", "acme"); err != nil {
		t.Fatalf("empty want must skip the check, got: %v", err)
	}
	err := VerifyWorkspace("acme", "evil")
	if !errors.Is(err, ErrWorkspaceMismatch) {
		t.Fatalf("cross-tenant access allowed: %v", err)
	}
	if strings.Contains(err.Error(), "evil") {
		t.Fatalf("mismatch error leaks the served tenant id: %q", err)
	}
}
