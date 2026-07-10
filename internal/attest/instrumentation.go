package attest

import (
	"github.com/trilam/leah/internal/obs"
)

func EmitAttempt(registry *telemetry.Registry, scope, outcome string) {
	if registry == nil {
		return
	}
	registry.Counter("leah_attestation_attempts_total").Inc(map[string]string{
		"scope":   scope,
		"outcome": outcome,
	})
}
