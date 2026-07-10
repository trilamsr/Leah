package contracts

import "context"

// Attestor gates a privileged operation behind operator consent.
// Adapter packages call Attestor.Attest(ctx, scope) BEFORE any
// secret-touching or external-side-effect step.
//
// Returned error means denial — adapters MUST treat denial as fatal
// for the current operation; NEVER fall back to non-attested action.
type Attestor interface {
	Attest(ctx context.Context, scope string) error
}
