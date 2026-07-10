package contracts

import "context"

// TokenSource yields a fresh bearer token on demand.
// Tests inject fakes; production uses file-backed sources that
// implement refresh-on-expiry logic.
type TokenSource interface {
	Token(ctx context.Context) (string, error)
}
