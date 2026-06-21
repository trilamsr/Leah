package connect

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"golang.org/x/oauth2"
)

// RefreshingSource satisfies the adapters' TokenSource without leaking oauth2.
type RefreshingSource struct {
	src oauth2.TokenSource
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
	return &RefreshingSource{src: cfg.TokenSource(ctx, &tok)}, nil
}

func (r *RefreshingSource) Token(_ context.Context) (string, error) {
	tok, err := r.src.Token()
	if err != nil {
		return "", fmt.Errorf("connect: token refresh: %w", err)
	}
	return tok.AccessToken, nil
}
