package contracts_test

import (
	"testing"

	commsout "github.com/trilam/leah/internal/actions/commsout"
	"github.com/trilam/leah/internal/platform/contracts"
)

// Compile-time check that the interfaces have no surprise method changes.
// If a wider interface lands here, this test will fail to compile.
var (
	_ contracts.Attestor
	_ contracts.TokenSource
	_ contracts.OSExec
	_ contracts.HTTPClient
	_ contracts.Notifier = (*commsout.Fanout)(nil)
	_ contracts.Notifier = (*commsout.Desktop)(nil)
	_ contracts.Notifier = (*commsout.Pushover)(nil)
	_ contracts.Notifier = (*commsout.VoiceNotify)(nil)
)

func TestContracts_Package_Importable(t *testing.T) {
	// If this test runs, the package compiled and is importable. That is the contract.
}
