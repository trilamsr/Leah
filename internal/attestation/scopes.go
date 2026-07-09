package attestation

// Canonical scope identifiers. String literals previously appeared at every
// Pool.Load callsite; centralising them here keeps audit-row Kind values and
// pool-registration calls in lockstep across packages.
const (
	// ScopeSelfBuild gates the GitHub-PR merge attestation (existing).
	ScopeSelfBuild = "self-build"

	// ScopeSelfBuildA2A gates inbound A2A self_build task delegation.
	// Distinct from ScopeSelfBuild so habituation on one cannot satisfy the
	// other and audit rows stay separable.
	ScopeSelfBuildA2A = "self-build-a2a"

	// CostOverrideScope guards the `leah cost override` flow (llm-ops spec §7.3).
	CostOverrideScope = "cost_override"

	// ScopeSelfUpgrade gates `leah self-upgrade`; distinct from ScopeSelfBuild so PR-merge habituation can't authorize a silent binary swap.
	ScopeSelfUpgrade = "self-upgrade"

	// ScopeShipTool gates `leah ship --<tool>` outward writes (post/create in
	// Slack, Jira, Notion, Linear, Teams). Distinct scope so habituation on the
	// PR-merge or binary-swap gates can't authorize an irreversible external post.
	ScopeShipTool = "ship-tool"

	// ScopeInboundEnroll gates the one-time loopback authorization that lets a
	// remote (channel,peer) pair answer pushed recommendations at all (F3 §4.2
	// layer 1). Distinct from any per-action scope so granting "this channel
	// may speak" never satisfies "this rec may run".
	ScopeInboundEnroll = "inbound-enroll"

	// ScopeInboundApply gates a single accept-from-remote that mutates state
	// for a `TierConfirm` rec without a stronger own-scope (F3 §4.2 layer 2).
	// Self-build / self-upgrade recs MUST attest their own scope instead —
	// remote origin never downgrades the gate.
	ScopeInboundApply = "inbound-apply"
)

// AllScopes lists every registered attestation scope. Wire one Pool with this
// slice and every authorised callsite picks from the same questions file.
func AllScopes() []string {
	return []string{ScopeSelfBuild, ScopeSelfBuildA2A, CostOverrideScope, ScopeSelfUpgrade, ScopeShipTool, ScopeInboundEnroll, ScopeInboundApply}
}
