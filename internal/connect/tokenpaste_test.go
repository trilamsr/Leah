package connect

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/oauth2"
)

// TestDefaultRegistry_WorkToolsResolvable proves the 6 work-tool adapters are
// connectable — Lookup must NOT return ErrUnknownProvider for any of them.
func TestDefaultRegistry_WorkToolsResolvable(t *testing.T) {
	reg := DefaultRegistry()
	for _, name := range []string{"confluence", "jira", "slack", "notion", "linear", "msteams"} {
		if _, err := reg.Lookup(name); err != nil {
			t.Fatalf("Lookup(%q) = %v, want resolvable provider", name, err)
		}
	}
}

// TestTokenPaste_WritesTokenMatchingAdapterHeader pins each tool's stored
// AccessToken to the exact value its adapter's Authorization header expects:
// wrong creds = dead integration.
func TestTokenPaste_WritesTokenMatchingAdapterHeader(t *testing.T) {
	b64 := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
	cases := []struct {
		name      string
		stdin     string
		wantToken string
	}{
		{"confluence", "ops@acme.com\nctok123\n", "Basic " + b64("ops@acme.com:ctok123")},
		{"jira", "ops@acme.com\njtok456\n", "Basic " + b64("ops@acme.com:jtok456")},
		{"slack", "xoxb-bot-789\n", "xoxb-bot-789"},
		{"notion", "secret_abc\n", "secret_abc"},
		{"linear", "lin_api_xyz\n", "lin_api_xyz"},
		{"msteams", "msteams-bearer\n", "msteams-bearer"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			tokenPath := filepath.Join(dir, tc.name+"-token.json")

			// Mirror the shipped provider's field set + assembly, redirected to a
			// temp token path and fed from a fake stdin.
			shipped := DefaultRegistry().byName[tc.name].(*tokenPasteProvider)
			p := &tokenPasteProvider{
				name:      shipped.name,
				tokenPath: tokenPath,
				fields:    shipped.fields,
				assemble:  shipped.assemble,
				in:        strings.NewReader(tc.stdin),
				out:       &bytes.Buffer{},
			}

			if _, err := Authorize(context.Background(), p, &fakeAttestor{}, nil); err != nil {
				t.Fatalf("Authorize: %v", err)
			}
			st, err := os.Stat(tokenPath)
			if err != nil {
				t.Fatalf("stat: %v", err)
			}
			if st.Mode().Perm() != 0o600 {
				t.Fatalf("mode: %v, want 0600", st.Mode().Perm())
			}
			b, _ := os.ReadFile(tokenPath)
			var tok oauth2.Token
			if err := json.Unmarshal(b, &tok); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if tok.AccessToken != tc.wantToken {
				t.Fatalf("AccessToken = %q, want %q", tok.AccessToken, tc.wantToken)
			}
		})
	}
}
