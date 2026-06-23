package budget

// DegradeAction is the ladder rung selected by spec §8.4 for a given
// (bucket, fillRatio) pair.
type DegradeAction int

const (
	ActionNone DegradeAction = iota
	ActionWarn
	ActionSwitchLocal
	ActionPause
	ActionBlock
)

// DegradePath implements the §8.4 table. Ratio is spent/cap.
//
// <80%      → none
// 80% to <100% → warn (soft surface; charge still allowed)
// >=100%    → bucket-specific terminal rung (switch-local / pause / block).
//
// The terminal rung at exactly cap matches "At 100%" column; "At cap exceeded"
// in the spec collapses to the same action for every row in the table where a
// local fallback exists, so we keep one rung here.
func DegradePath(b Bucket, ratio float64) DegradeAction {
	switch {
	case ratio < 0.80:
		return ActionNone
	case ratio < 1.00:
		return ActionWarn
	}
	switch b {
	case BucketCloudEmbedBytes,
		BucketCloudSTTSeconds,
		BucketCloudTTSChars,
		BucketCloudVisionBytes:
		return ActionSwitchLocal
	case BucketPluginNetworkBytes:
		return ActionPause
	default:
		return ActionBlock
	}
}
