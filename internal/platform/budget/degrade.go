package budget

// DegradeAction is the ladder rung selected by spec §8.4 for a given
// (bucket, fillRatio) pair. Disable is the "At cap exceeded" terminal rung
// distinct from Pause/Block at exactly-100%; the spec separates them for
// plugin (pause 60s → disable until reset) and peer.a2a (warning → exhausted).
type DegradeAction int

const (
	ActionNone DegradeAction = iota
	ActionWarn
	ActionSwitchLocal
	ActionPause
	ActionBlock
	ActionDisable
)

// DegradePath implements the §8.4 table column-by-column. Ratio is spent/cap.
func DegradePath(b Bucket, ratio float64) DegradeAction {
	switch {
	case ratio < 0.80:
		return ActionNone
	case ratio < 1.00:
		return ActionWarn
	case ratio < 1.0001:
		// At 100% column.
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
	default:
		// At cap exceeded — local-fallback buckets stay switched, peer/plugin escalate.
		switch b {
		case BucketCloudEmbedBytes,
			BucketCloudSTTSeconds,
			BucketCloudTTSChars,
			BucketCloudVisionBytes:
			return ActionSwitchLocal
		case BucketPluginNetworkBytes:
			return ActionDisable
		default:
			return ActionBlock
		}
	}
}
