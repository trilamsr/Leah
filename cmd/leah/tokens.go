package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"

	"github.com/trilam/leah/internal/connect"
)

// noopAttestor satisfies each adapter's per-RPC gate; the CLI-level consent
// (per-subcommand) is the operator gate, not the adapter.
type noopAttestor struct{}

func (noopAttestor) Attest(context.Context, string) error { return nil }

// staticToken is a fixed-value TokenSource for adapters whose credentials are
// pasted once (no OAuth refresh) — the token was written to disk by
// `leah connect <name>` and read back here on each command invocation.
type staticToken string

func (t staticToken) Token(context.Context) (string, error) {
	if t == "" {
		return "", errors.New("not connected: run leah connect first")
	}
	return string(t), nil
}

// staticTokenForName loads the AccessToken from the connect-written token file
// at ~/.leah-state/secrets/<name>-token.json. Missing/unreadable/malformed
// returns "" so the caller's staticToken.Token surfaces the "not connected"
// error at first use.
func staticTokenForName(name string) staticToken {
	buf, err := os.ReadFile(connect.DefaultTokenPath(name))
	if err != nil {
		return ""
	}
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if json.Unmarshal(buf, &tok) != nil {
		return ""
	}
	return staticToken(tok.AccessToken)
}
