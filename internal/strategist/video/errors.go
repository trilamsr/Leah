package video

import "errors"

// Sentinels callers use with errors.Is. They map 1:1 to the spec's
// strategist error-handling rows: ffmpeg-missing → doctor hint; timeout →
// OutcomeQueued (clip_unavailable); adapter-failed → wraps the adapter's
// own sentinel (rate-limit / policy-refused) so the CLI dispatches by Is.
var (
	ErrTimeout       = errors.New("video: clip poll deadline reached")
	ErrFFmpegMissing = errors.New("video: ffmpeg not found in PATH (run `brew install ffmpeg`)")
	ErrAdapterFailed = errors.New("video: higgsfield adapter failed")
)
