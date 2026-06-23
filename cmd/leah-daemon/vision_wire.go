package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/trilam/leah/internal/budget"
	"github.com/trilam/leah/internal/vision"
	"github.com/trilam/leah/internal/vision/ocr"
	"github.com/trilam/leah/internal/vision/router"
)

// notWiredSonnet is the fallback cloud vision leg used when no ANTHROPIC_API_KEY
// is present at daemon start. It emits one terminal Err chunk so the HUD shows
// a real failure instead of silently truncating.
type notWiredSonnet struct{}

func (notWiredSonnet) StreamVision(_ context.Context, _ vision.Image, _ string) (<-chan router.VisionChunk, error) {
	ch := make(chan router.VisionChunk, 1)
	ch <- router.VisionChunk{Err: errors.New("vision: sonnet not wired (ANTHROPIC_API_KEY unset)")}
	close(ch)
	return ch, nil
}

// visionMeter adapts budget.Runtime to router.VisionMeter. The router charges
// against a string bucket key (kept SDK-free); budget.Runtime takes typed
// budget.Bucket. Identity cast suffices because router.BucketCloudVisionBytes
// is the verbatim mirror of budget.BucketCloudVisionBytes.
type visionMeter struct{ r budget.Runtime }

func (m visionMeter) Charge(ctx context.Context, bucket string, n int64) error {
	return m.r.Charge(ctx, budget.Bucket(bucket), n)
}

// wireVisionRouter constructs the production vision Router with budget
// metering against the daemon DB. Sonnet remains the NotWired placeholder
// until T04 followups ship a real Anthropic vision client; consent is the
// in-memory store (DB-backed wrapper arrives with T04 followups). Prompt is
// a deny stub — vision live modes stay default-OFF until the HUD wires a
// real prompt.
//
// Returns a non-nil router even when budget.NewRuntime fails: router.New
// accepts a nil meter and skips the Charge step, so the cloud path surfaces
// a terminal Sonnet error rather than a swallowed nil-panic.
func wireVisionRouter(db *sql.DB, errOut io.Writer, lg *slog.Logger) router.Router {
	var meter router.VisionMeter
	rt, err := budget.NewRuntime(db)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "leah-daemon: vision budget runtime non-fatal: %v\n", err)
	} else {
		meter = visionMeter{r: rt}
	}
	var sonnet router.SonnetClient = notWiredSonnet{}
	sonnetState := "not_wired"
	if c, cerr := router.NewSonnetClient(); cerr == nil {
		sonnet = c
		sonnetState = "anthropic"
	}
	r := router.New(
		ocr.NewEngine(),
		sonnet,
		router.NewMemConsent(),
		meter,
		func(string) bool { return false },
	)
	lg.Info("vision.router.wired",
		"ocr", "darwin",
		"sonnet", sonnetState,
		"consent", "mem",
		"meter", meter != nil,
		"prompt", "deny_stub",
	)
	return r
}
