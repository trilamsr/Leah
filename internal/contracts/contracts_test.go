package contracts_test

import (
	"testing"

	"github.com/trilam/leah/internal/contracts"
	"github.com/trilam/leah/internal/notify"
)

// Compile-time check that the interfaces have no surprise method changes.
// If a wider interface lands here, this test will fail to compile.
var (
	_ contracts.Attestor
	_ contracts.TokenSource
	_ contracts.OSExec
	_ contracts.HTTPClient
	_ contracts.Notifier = (*notify.Fanout)(nil)
	_ contracts.Notifier = (*notify.Desktop)(nil)
	_ contracts.Notifier = (*notify.Pushover)(nil)
	_ contracts.Notifier = (*notify.VoiceNotify)(nil)
)

func TestContracts_Package_Importable(t *testing.T) {
	// If this test runs, the package compiled and is importable. That is the contract.
}
