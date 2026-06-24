package connect

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMCPFallback_PersistsEndpoint pins the contract: a valid http(s) URL
// pasted by the operator is stored verbatim as AccessToken so the runtime
// MCP host can dial it. Anything weaker (host-only, ssh URL) is rejected.
func TestMCPFallback_PersistsEndpoint(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)

	var out bytes.Buffer
	p := newMCPFallback("zapier", strings.NewReader("https://mcp.zapier.com/sse\n"), &out)
	tok, err := Authorize(context.Background(), p, &fakeAttestor{}, nopPrompt)
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	if tok.AccessToken != "https://mcp.zapier.com/sse" || tok.TokenType != "mcp" {
		t.Fatalf("token: %+v", tok)
	}
	if !strings.Contains(out.String(), "MCP endpoint URL") {
		t.Fatalf("prompt missing: %q", out.String())
	}
	st, err := os.Stat(filepath.Join(dir, "secrets", "zapier-token.json"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("mode: %v", st.Mode().Perm())
	}
}

// TestMCPFallback_RejectsBadInput covers empty input + non-http schemes —
// reject up front; a bad endpoint would only surface at first adapter call.
func TestMCPFallback_RejectsBadInput(t *testing.T) {
	cases := []struct {
		name, input string
	}{
		{"empty", "\n"},
		{"bare-host", "mcp.zapier.com\n"},
		{"ssh-scheme", "ssh://mcp.example.com\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("LEAH_STATE_DIR", t.TempDir())
			var out bytes.Buffer
			p := newMCPFallback("x", strings.NewReader(c.input), &out)
			_, err := Authorize(context.Background(), p, &fakeAttestor{}, nopPrompt)
			if err == nil {
				t.Fatalf("want error, got nil")
			}
			if errors.Is(err, ErrAttestationDenied) {
				t.Fatalf("rejected via attestation, not validation: %v", err)
			}
		})
	}
}
