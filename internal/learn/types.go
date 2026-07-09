// Package learn wraps the Thompson kernel in the recommendation pass-2
// lifecycle: confidence floor, pacing caps, and decay.
package learn

import (
	"errors"
	"time"
)

type RecommendKind string
type Surface string
type ActionRef string
type RecommendationID int64
type AntiSource string

const (
	SurfaceNotification Surface = "notification"
	SurfaceVoiceClose   Surface = "voice_close"
	SurfaceCoachCard    Surface = "coach"

	AntiOperator AntiSource = "operator"
	AntiAuto     AntiSource = "auto"
	AntiSpec     AntiSource = "spec"

	ConfidenceFloor = 0.35
	PacingPerHour   = 1
	PacingPerDay    = 3

	// Spec §3.6: 3 Dismissed in 30 d auto-adds to anti-list.
	AutoDismissThreshold = 3
	AutoDismissWindow    = 30 * 24 * time.Hour

	// Spec §3.7: lock arm once each side hits this impression count.
	ABLockThreshold = 50
)

// ErrSpecLocked rejects operator/auto removal of a spec-pinned anti-rule (§3.9).
var ErrSpecLocked = errors.New("learn: anti-list spec row is immutable from this source")

type Observation struct {
	Kind    RecommendKind
	CtxHash uint64
	Ts      time.Time
}

type Recommendation struct {
	ID         RecommendationID
	Kind       RecommendKind
	Body       string
	Score      float64
	Confidence float64
	Action     ActionRef
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

type AntiRule struct {
	Kind    RecommendKind
	Reason  string
	Source  AntiSource
	AddedAt time.Time
}

type Experiment struct {
	ID           int64
	Kind         RecommendKind
	ArmA, ArmB   string
	ImpressionsA int
	ImpressionsB int
	WinsA        int
	WinsB        int
	Locked       bool
	LockedArm    string
}
