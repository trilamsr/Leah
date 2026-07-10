package budget

// DefaultBucket is the spec §8.2 default (cap, window) for a bucket. A zero
// Cap means "unlimited" (BYOK ledger only) — Charge skips the cap check.
type DefaultBucket struct {
	Bucket Bucket
	Cap    int64
	Window Window
}

// Defaults returns the §8.2 default cap table verbatim. Caller seeds these
// into budget_bucket on first daemon start; operator edits persist in the DB
// and Defaults is never re-applied (so a lowered cap stays lowered).
func Defaults() []DefaultBucket {
	return []DefaultBucket{
		{BucketCloudLLMTokens, 0, WindowDay}, // unlimited (BYOK ledger only)
		{BucketCloudEmbedBytes, 50 * 1024 * 1024, WindowDay},
		{BucketCloudSTTSeconds, 900, WindowDay}, // 15 min
		{BucketCloudTTSChars, 25_000, WindowDay},
		{BucketCloudVisionBytes, 30 * 1024 * 1024, WindowDay},
		{BucketPeerA2ATokens, 5_000, WindowDay},
		{BucketPluginNetworkBytes, 50 * 1024 * 1024, WindowDay},
	}
}
