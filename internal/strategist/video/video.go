// Package video orchestrates a Higgsfield text-to-clip job end-to-end:
// submit → poll-to-ready → fetch mp4 → ffmpeg caption-burn + aspect
// reformat → persist into the strategist item's media slot. macOS-only
// in v1 (ffmpeg is expected at /opt/homebrew/bin/ffmpeg or /usr/local/bin
// after `brew install ffmpeg`); the doctor command surfaces the gap.
package video

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Ratio constants. Spec: 9:16 vertical for Reels/TikTok, 16:9 landscape
// for LinkedIn/Facebook. The string IS the value the ffmpeg runner
// matches on — keep them stable.
const (
	Ratio9x16 = "9:16"
	Ratio16x9 = "16:9"
)

// HiggsfieldClient is the subset of *higgsfield.Adapter the orchestrator
// needs. Defined here (not imported) so tests fake without pulling
// attestation/token plumbing into the unit test.
type HiggsfieldClient interface {
	SubmitClip(ctx context.Context, prompt string, seconds int) (jobID string, err error)
	PollClip(ctx context.Context, jobID string) (status string, mp4 []byte, err error)
}

// FFmpegRunner abstracts the local ffmpeg binary. Production impl in
// ffmpeg.go; tests inject a recorder.
type FFmpegRunner interface {
	Available() bool
	BurnCaption(ctx context.Context, in, out, text, ratio string) error
}

// HTTPGetter fetches bytes from a URL — separate from the adapter so the
// orchestrator can swap to direct-bytes (current Higgsfield Transport
// signature) without churning the seam. Currently unused in the happy
// path (Transport.PollClip returns mp4 bytes inline) but retained for
// the future presigned-URL response shape.
type HTTPGetter interface {
	Get(ctx context.Context, url string) ([]byte, error)
}

// ItemFS abstracts the assets directory layout so tests verify path
// shape without touching the real filesystem. ReserveOutput returns the
// final destination path (e.g. <stateDir>/strategist/assets/<id>/clip.mp4)
// and WriteTemp stages intermediate bytes.
type ItemFS interface {
	WriteTemp(itemID, name string, b []byte) (string, error)
	ReserveOutput(itemID, name string) (string, error)
	Persist(src, dst string) error
	Remove(path string)
}

// ItemRef identifies the strategist item whose media slot we fill. The
// orchestrator is decoupled from store.Item — the slot path is derived
// from ID alone, matching the spec's `<stateDir>/strategist/assets/<id>/`
// convention.
type ItemRef struct {
	ID string
}

// ClipRequest is the orchestrator input. Caption may be empty (skip
// burn-in). Ratio defaults to 9:16 if zero.
type ClipRequest struct {
	Prompt  string
	Seconds int
	Caption string
	Ratio   string
}

// Config wires the four collaborators + clock + poll timing.
type Config struct {
	Adapter      HiggsfieldClient
	FFmpeg       FFmpegRunner
	Fetcher      HTTPGetter
	FS           ItemFS
	PollInterval time.Duration
	PollDeadline time.Duration
	Now          func() time.Time
}

// Orchestrator composes one clip per Compose call. Concurrent calls are
// safe; state is per-invocation.
type Orchestrator struct {
	a   HiggsfieldClient
	ff  FFmpegRunner
	fe  HTTPGetter
	fs  ItemFS
	iv  time.Duration
	dl  time.Duration
	now func() time.Time
}

// New applies defaults: 10s poll interval + 5m deadline (spec row
// "clip poll deadline reached"); zero values in cfg fall back.
func New(cfg Config) *Orchestrator {
	iv := cfg.PollInterval
	if iv <= 0 {
		iv = 10 * time.Second
	}
	dl := cfg.PollDeadline
	if dl <= 0 {
		dl = 5 * time.Minute
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Orchestrator{
		a:   cfg.Adapter,
		ff:  cfg.FFmpeg,
		fe:  cfg.Fetcher,
		fs:  cfg.FS,
		iv:  iv,
		dl:  dl,
		now: now,
	}
}

// Compose drives the end-to-end pipeline. Returns the path the operator
// reads from the item's `clip:` front-matter slot.
func (o *Orchestrator) Compose(ctx context.Context, ref ItemRef, req ClipRequest) (string, error) {
	if req.Ratio == "" {
		req.Ratio = Ratio9x16
	}
	// WHY: pre-flight LookPath catches the "operator forgot brew install
	// ffmpeg" path BEFORE charging a Higgsfield call. doctor surfaces the
	// same gap.
	if req.Caption != "" && !o.ff.Available() {
		return "", ErrFFmpegMissing
	}

	jobID, err := o.a.SubmitClip(ctx, req.Prompt, req.Seconds)
	if err != nil {
		return "", fmt.Errorf("%w: submit: %v", ErrAdapterFailed, err)
	}

	mp4, err := o.pollUntilReady(ctx, jobID)
	if err != nil {
		return "", err
	}

	// WHY: bytes go through a staging temp so ffmpeg has a stable -i
	// path; Persist atomically lands the final output regardless of
	// burn-in vs raw path.
	rawPath, err := o.fs.WriteTemp(ref.ID, "raw.mp4", mp4)
	if err != nil {
		return "", fmt.Errorf("video: write raw temp: %w", err)
	}
	defer o.fs.Remove(rawPath)

	dst, err := o.fs.ReserveOutput(ref.ID, "clip.mp4")
	if err != nil {
		return "", fmt.Errorf("video: reserve output: %w", err)
	}

	if req.Caption == "" {
		// WHY: caption-less still needs the aspect reformat in the
		// general case, but v1 ships caption-required for short-form
		// social so empty-caption = keep raw bytes.
		if err := o.fs.Persist(rawPath, dst); err != nil {
			return "", fmt.Errorf("video: persist raw: %w", err)
		}
		return dst, nil
	}

	burnedPath, err := o.fs.WriteTemp(ref.ID, "burned.mp4", nil)
	if err != nil {
		return "", fmt.Errorf("video: write burned temp: %w", err)
	}
	defer o.fs.Remove(burnedPath)

	if err := o.ff.BurnCaption(ctx, rawPath, burnedPath, req.Caption, req.Ratio); err != nil {
		if errors.Is(err, ErrFFmpegMissing) {
			return "", err
		}
		return "", fmt.Errorf("video: ffmpeg burn: %w", err)
	}
	if err := o.fs.Persist(burnedPath, dst); err != nil {
		return "", fmt.Errorf("video: persist: %w", err)
	}
	return dst, nil
}

// pollUntilReady loops at o.iv until the adapter reports "ready" or the
// deadline fires. Returns ErrTimeout on deadline. ctx cancellation wins
// over the deadline so a stuck call doesn't pin the goroutine.
func (o *Orchestrator) pollUntilReady(ctx context.Context, jobID string) ([]byte, error) {
	deadline := o.now().Add(o.dl)
	ticker := time.NewTicker(o.iv)
	defer ticker.Stop()
	for {
		status, mp4, err := o.a.PollClip(ctx, jobID)
		if err != nil {
			return nil, fmt.Errorf("%w: poll: %v", ErrAdapterFailed, err)
		}
		if strings.EqualFold(status, "ready") {
			return mp4, nil
		}
		if o.now().After(deadline) {
			return nil, ErrTimeout
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}
