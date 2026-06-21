package attestation

// Canonical scope identifiers. String literals previously appeared at every
// Pool.Load callsite; centralising them here keeps audit-row Kind values and
// pool-registration calls in lockstep across packages.
const (
	// ScopeSelfBuild gates the GitHub-PR merge attestation (existing).
	ScopeSelfBuild = "self-build"

	// ScopeSelfBuildA2A gates inbound A2A self_build task delegation (W139).
	// Distinct from ScopeSelfBuild so habituation on one cannot satisfy the
	// other and audit rows stay separable (spec §2.3).
	ScopeSelfBuildA2A = "self-build-a2a"

	// CostOverrideScope guards the `leah cost override` flow (llm-ops spec §7.3).
	CostOverrideScope = "cost_override"

	// ScopeSelfUpgrade gates `leah self-upgrade`; distinct from ScopeSelfBuild so PR-merge habituation can't authorize a silent binary swap.
	ScopeSelfUpgrade = "self-upgrade"
)

// AllScopes lists every registered attestation scope. Wire one Pool with this
// slice and every authorised callsite picks from the same questions file.
func AllScopes() []string {
	return []string{ScopeSelfBuild, ScopeSelfBuildA2A, CostOverrideScope, ScopeSelfUpgrade}
}
