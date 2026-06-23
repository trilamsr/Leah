// Package learn wraps the Phase 3 Thompson kernel in the recommendation
// pass-2 lifecycle: confidence floor, pacing caps, and §3.5 decay.
package learn

import "time"

type RecommendKind string
type Surface string

const (
	SurfaceNotification Surface = "notification"
	SurfaceVoiceClose   Surface = "voice_close"
	SurfaceCoachCard    Surface = "coach"

	ConfidenceFloor = 0.35
	PacingPerHour   = 1
	PacingPerDay    = 3
)

type Observation struct {
	Kind    RecommendKind
	CtxHash uint64
	Ts      time.Time
}

type Recommendation struct {
	ID         int64
	Kind       RecommendKind
	Body       string
	Score      float64
	Confidence float64
	ActionRef  string
	SurfacedAt time.Time
	ExpiresAt  time.Time
}

type OutcomeKind int

const (
	Accepted OutcomeKind = iota
	Dismissed
	Ignored
	Applied
	ABBaseline
	ABTreatment
)

type Outcome struct {
	Kind      OutcomeKind
	LatencyMS int
	Note      string
}
