package connect

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"golang.org/x/oauth2"
)

// RefreshingSource satisfies the adapters' TokenSource without leaking oauth2.
type RefreshingSource struct {
	src  oauth2.TokenSource
	path string

	mu   sync.Mutex
	last oauth2.Token
}

func LoadRefreshingSource(ctx context.Context, clientID, clientSecret, path string) (*RefreshingSource, error) {
	buf, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("connect: read token: %w", err)
	}
	var tok oauth2.Token
	if err := json.Unmarshal(buf, &tok); err != nil {
		return nil, fmt.Errorf("connect: parse token: %w", err)
	}
	cfg := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     oauth2.Endpoint{TokenURL: googleTokenURL},
	}
	return &RefreshingSource{src: cfg.TokenSource(ctx, &tok), path: path, last: tok}, nil
}

// StaticSource yields a fixed paste-token credential (jira/linear/notion/
// confluence) read once from disk — no refresh, mirroring the no-OAuth
// tokenPasteProvider that wrote it.
type StaticSource struct{ token string }

// LoadStaticSource reads the AccessToken from a paste-token file at path.
func LoadStaticSource(path string) (*StaticSource, error) {
	buf, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("connect: read token: %w", err)
	}
	var tok oauth2.Token
	if err := json.Unmarshal(buf, &tok); err != nil {
		return nil, fmt.Errorf("connect: parse token: %w", err)
	}
	if tok.AccessToken == "" {
		return nil, fmt.Errorf("connect: empty token at %s", path)
	}
	return &StaticSource{token: tok.AccessToken}, nil
}

func (s *StaticSource) Token(_ context.Context) (string, error) { return s.token, nil }

func (r *RefreshingSource) Token(_ context.Context) (string, error) {
	tok, err := r.src.Token()
	if err != nil {
		return "", fmt.Errorf("connect: token refresh: %w", err)
	}
	r.persistIfRotated(tok)
	return tok.AccessToken, nil
}

// persistIfRotated re-writes the on-disk token when the underlying source
// rotated the access or refresh token; without this a Google-rotated
// refresh_token is lost on daemon restart and the integration silently dies.
func (r *RefreshingSource) persistIfRotated(tok *oauth2.Token) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if tok.AccessToken == r.last.AccessToken &&
		tok.RefreshToken == r.last.RefreshToken &&
		tok.Expiry.Equal(r.last.Expiry) {
		return
	}
	if r.path == "" {
		return
	}
	if err := WriteToken(r.path, tok); err != nil {
		return
	}
	r.last = *tok
}
