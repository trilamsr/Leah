package video

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// DefaultFFmpegRunner shells out to the local ffmpeg binary. Argv is
// constructed deterministically (no shell interpolation) and the caption
// is sanitized BEFORE composition so filtergraph metachars never reach
// the child process — argv-injection vectors are closed at the source.
//
// lookPath/runCmd are seams so tests assert argv without spawning ffmpeg.
type DefaultFFmpegRunner struct {
	lookPath func(string) (string, error)
	runCmd   func(ctx context.Context, name string, args []string) error
}

// NewDefaultFFmpegRunner returns a runner that talks to the real binary.
// Tests in this package construct DefaultFFmpegRunner directly with
// scripted seams.
func NewDefaultFFmpegRunner() *DefaultFFmpegRunner {
	return &DefaultFFmpegRunner{
		lookPath: exec.LookPath,
		runCmd: func(ctx context.Context, name string, args []string) error {
			// WHY: CommandContext kills the child on ctx cancel so a
			// stuck encode doesn't outlive the Compose call.
			c := exec.CommandContext(ctx, name, args...)
			c.WaitDelay = 2 // seconds; ensure Kill if the child ignores SIGTERM
			return c.Run()
		},
	}
}

// Available returns whether ffmpeg is on PATH. Cheap call; safe to use
// as a pre-flight in Compose.
func (r *DefaultFFmpegRunner) Available() bool {
	_, err := r.lookPath("ffmpeg")
	return err == nil
}

// BurnCaption composes one filtergraph that reformats to ratio + draws
// the caption text on top. drawtext is chosen over subtitles because
// subtitles= reads a file path (extra temp file, more failure modes);
// drawtext takes a literal expression we control via sanitizeCaption.
func (r *DefaultFFmpegRunner) BurnCaption(ctx context.Context, in, out, text, ratio string) error {
	bin, err := r.lookPath("ffmpeg")
	if err != nil {
		return ErrFFmpegMissing
	}
	w, h := dims(ratio)
	caption := sanitizeCaption(text)
	// drawtext expression: caption white, semi-opaque box, centered, near
	// the bottom for thumb-safe placement.
	filter := fmt.Sprintf(
		"scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d,drawtext=text='%s':fontcolor=white:fontsize=42:box=1:boxcolor=black@0.5:boxborderw=12:x=(w-text_w)/2:y=h-(text_h*3)",
		w, h, w, h, caption,
	)
	args := []string{
		"-y",
		"-i", in,
		"-vf", filter,
		"-c:a", "copy",
		"-movflags", "+faststart",
		out,
	}
	if err := r.runCmd(ctx, bin, args); err != nil {
		return fmt.Errorf("ffmpeg: %w", err)
	}
	return nil
}

// dims returns the target (width, height) for the named aspect ratio.
// 1080p short side keeps file sizes reasonable for social uploads.
func dims(ratio string) (int, int) {
	switch ratio {
	case Ratio16x9:
		return 1920, 1080
	default:
		// WHY: 9:16 is the default; any unknown value falls through
		// here rather than failing — Compose already validated ratio
		// against the two constants in practice.
		return 1080, 1920
	}
}

// sanitizeCaption strips the metachars ffmpeg drawtext treats specially
// in single-quoted expressions: `;` (filtergraph separator), `:` (option
// separator), `'` (quote terminator), `\` (escape), backtick + newlines
// (shell habits that often slip in from operator copy-paste). The result
// is safe to interpolate inside text='...' without an injection vector.
func sanitizeCaption(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case ';', ':', '\'', '\\', '`', '\n', '\r', '\t':
			b.WriteRune(' ')
		default:
			if r < 0x20 {
				b.WriteRune(' ')
				continue
			}
			b.WriteRune(r)
		}
	}
	// Collapse runs of spaces produced by the substitution + trim.
	out := strings.Join(strings.Fields(b.String()), " ")
	return out
}
