package learn

import (
	"math"
	"time"
)

type DecaySchedule struct {
	Kind       RecommendKind
	HalfLife   time.Duration
	HardExpire time.Duration
}

func Decay(score float64, age time.Duration, s DecaySchedule) float64 {
	if s.HardExpire > 0 && age > s.HardExpire {
		return 0
	}
	if s.HalfLife <= 0 {
		return score
	}
	ratio := float64(age) / float64(s.HalfLife)
	return score * math.Pow(0.5, ratio)
}
