package budget

import "time"

// Bucket names a privacy-budget meter from §8.2. Values are stable strings —
// they appear in the budget_bucket DB rows and in operator-visible Settings.
type Bucket string

// Window is the rollover cadence for a bucket. Stored verbatim in
// budget_bucket.window per §8.5.
type Window string

const (
	WindowHour  Window = "hour"
	WindowDay   Window = "day"
	WindowWeek  Window = "week"
	WindowMonth Window = "month"
)

const (
	BucketCloudLLMTokens     Bucket = "cloud.llm.tokens"
	BucketCloudEmbedBytes    Bucket = "cloud.embed.bytes"
	BucketCloudSTTSeconds    Bucket = "cloud.stt.seconds"
	BucketCloudTTSChars      Bucket = "cloud.tts.chars"
	BucketCloudVisionBytes   Bucket = "cloud.vision.bytes"
	BucketPeerA2ATokens      Bucket = "peer.a2a.tokens"
	BucketPluginNetworkBytes Bucket = "plugin.network.bytes"
)

// Balance is a Peek snapshot. Trend holds the last 24 samples (oldest→newest).
type Balance struct {
	Bucket   Bucket
	Spent    int64
	Cap      int64
	Window   Window
	ResetsAt time.Time
	Trend    []int64
}

// Event is published on Subscribe whenever a Charge mutates a bucket. Action
// is computed by DegradePath so subscribers can drive UI / fallback policy
// without re-deriving the ratio.
type Event struct {
	Bucket Bucket
	Spent  int64
	Cap    int64
	Action DegradeAction
}
