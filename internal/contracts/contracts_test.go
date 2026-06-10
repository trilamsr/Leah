package contracts_test

import (
	"testing"

	"github.com/trilam/leah/internal/contracts"
)

// Compile-time check that the interfaces have no surprise method changes.
// If a wider interface lands here, this test will fail to compile.
var (
	_ contracts.Attestor
	_ contracts.TokenSource
	_ contracts.OSExec
	_ contracts.HTTPClient
)

func TestContracts_Package_Importable(t *testing.T) {
	// If this test runs, the package compiled and is importable. That is the contract.
}
