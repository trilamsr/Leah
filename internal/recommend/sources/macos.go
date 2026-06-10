package sources

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	"github.com/trilam/leah/internal/recommend"
)

// Source is the W-this contract: each impl converts an external signal seam
// into recommend.Recommendation candidates. Wiring into Engine.Propose lands
// in a follow-up wave — sources here are read-only and side-effect free.
type Source interface {
	Name() string
	Recommendations(ctx context.Context) ([]recommend.Recommendation, error)
}

// AppEvent is the per-app-open observation projected from macOS-mirror.db.
// Kept minimal so the seam stays substitutable (real impl: SELECT from the
// mirror's app-events table; test impl: in-memory slice).
type AppEvent struct {
	App       string
	Timestamp time.Time
}

// MirrorSeam is the structural read-only contract MacosMirrorSource consumes.
// Anything that can list app-open events within a window satisfies it — the
// production wire-up lives in cmd/leah-daemon, not here.
type MirrorSeam interface {
	AppOpens(ctx context.Context, since time.Time) ([]AppEvent, error)
}

// MacosOpts controls cadence detection. Defaults match the cold-start carve-out
// (≥3 same-window opens over 14 days = pattern); tests inject Now.
type MacosOpts struct {
	Window      time.Duration // lookback; defaults to 14d
	MinOccur    int           // min same-app opens to fire; defaults to 3
	Now         func() time.Time
}

type MacosMirrorSource struct {
	seam MirrorSeam
	opts MacosOpts
}

func NewMacosMirrorSource(seam MirrorSeam, opts MacosOpts) *MacosMirrorSource {
	if opts.Window == 0 {
		opts.Window = 14 * 24 * time.Hour
	}
	if opts.MinOccur == 0 {
		opts.MinOccur = 3
	}
	if opts.Now == nil {
		opts.Now = func() time.Time { return time.Now().UTC() }
	}
	return &MacosMirrorSource{seam: seam, opts: opts}
}

func (s *MacosMirrorSource) Name() string { return "macos-mirror" }

// Recommendations bins app-opens by (app, hour-of-day) and surfaces any bucket
// crossing MinOccur. Confidence = count / window-days, clamped to (0, 1].
// Tier is TierConfirm — every cadence proposal stays operator-gated until the
// pattern adapter (W18+) ships its own rubric mapping.
func (s *MacosMirrorSource) Recommendations(ctx context.Context) ([]recommend.Recommendation, error) {
	since := s.opts.Now().Add(-s.opts.Window)
	events, err := s.seam.AppOpens(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("macos-mirror: %w", err)
	}
	type bucket struct {
		app  string
		hour int
		n    int
	}
	bins := make(map[string]*bucket)
	for _, e := range events {
		if e.Timestamp.Before(since) {
			continue
		}
		k := e.App + ":" + fmt.Sprintf("%02d", e.Timestamp.Hour())
		b, ok := bins[k]
		if !ok {
			b = &bucket{app: e.App, hour: e.Timestamp.Hour()}
			bins[k] = b
		}
		b.n++
	}
	windowDays := float64(s.opts.Window / (24 * time.Hour))
	if windowDays <= 0 {
		windowDays = 1
	}
	var out []recommend.Recommendation
	for k, b := range bins {
		if b.n < s.opts.MinOccur {
			continue
		}
		conf := float64(b.n) / windowDays
		if conf > 1 {
			conf = 1
		}
		pattern := fmt.Sprintf("macos.cadence.%s.h%02d", b.app, b.hour)
		out = append(out, recommend.Recommendation{
			ID:         hashID("macos", k, s.opts.Now()),
			Pattern:    pattern,
			Tier:       recommend.TierConfirm,
			Source:     s.Name(),
			Confidence: conf,
			CreatedAt:  s.opts.Now(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Confidence > out[j].Confidence })
	return out, nil
}

// hashID derives a deterministic id from the pattern key + source bucket so
// retried Propose calls converge on the same row. SHA-256 truncated to keep
// the column human-greppable.
func hashID(prefix, key string, t time.Time) string {
	h := sha256.Sum256([]byte(prefix + "|" + key + "|" + t.Format(time.RFC3339)))
	return prefix + "-" + hex.EncodeToString(h[:8])
}
