package connect

import (
	"context"
	"errors"
	"fmt"
	"os"
)

var ErrTokenFileNotFound = errors.New("connect: token file not found")

// Revoker is the optional interface a Provider may implement to perform a
// best-effort upstream token revoke before the local file is deleted. Failures
// are intentionally swallowed by Disconnect — local cleanup is the primary
// intent, the upstream call is courtesy.
type Revoker interface {
	Revoke(ctx context.Context) error
}

// Disconnect runs the inverse of Authorize: attestation FIRST, then optional
// upstream revoke (best-effort), then on-disk token removal. Ordering matches
// Authorize's "consent before side effects" rule — a denied attestation MUST
// NOT delete the token file.
func Disconnect(ctx context.Context, p Provider, att Attestor) error {
	if err := att.Attest(ctx, "disconnect:"+p.Name()); err != nil {
		return fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	if _, err := os.Stat(p.TokenPath()); errors.Is(err, os.ErrNotExist) {
		return ErrTokenFileNotFound
	}
	if r, ok := p.(Revoker); ok {
		_ = r.Revoke(ctx)
	}
	if err := os.Remove(p.TokenPath()); err != nil {
		return fmt.Errorf("connect: remove token: %w", err)
	}
	return nil
}
