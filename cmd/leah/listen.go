package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/dispatcher"
	"github.com/trilam/leah/internal/intent"
	"github.com/trilam/leah/internal/obs"
	"github.com/trilam/leah/internal/voice"
)

// runListen records one push-to-talk utterance, transcribes it via
// whisper.cpp locally, then routes the transcript through the existing
// intent classifier → CLI command dispatch.
//
// Routing rules (mirror the intent package):
//   - KindAsk    → runAsk(transcript)
//   - KindShip   → print transcript + prompt operator for repo (no
//     auto-pick — every ship is a load-bearing irreversible
//     action)
//   - KindReview → parse pr# from transcript + dispatch runReview
//   - KindStatus → forward to runStatus equivalent (audit-log printout)
//
// One audit row per invocation: kind=voice.input, BR=0, detail=truncated
// transcript + classified verb.
func runListen(ctx context.Context, reg *telemetry.Registry, args []string) int {
	fs := flag.NewFlagSet("listen", flag.ContinueOnError)
	duration := fs.Duration("duration", 0, "max recording length (e.g. 30s); 0 = silence-detector only")
	model := fs.String("model", "ggml-large-v3-turbo-q5_0.bin", "whisper.cpp model filename in --model-dir")
	modelDir := fs.String("model-dir", "models", "directory containing ggml-*.bin model files")
	repo := fs.String("repo", "", "repo to use when transcript classifies as ship/review")
	if err := fs.Parse(args); err != nil {
		fs.Usage()
		return 2
	}

	a := &audit.Logger{Path: filepath.Join(stateDir(), "audit.jsonl")}

	_, _ = fmt.Println("listening... (Ctrl-C to stop)")

	transcript, err := voice.Listen(ctx, voice.ShellExec{}, voice.ListenConfig{
		Duration: *duration,
		Model:    *model,
		ModelDir: *modelDir,
	})
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah listen: %v\n", err)
		return 1
	}
	if transcript == "" {
		_, _ = fmt.Fprintln(os.Stderr, "leah listen: empty transcript")
		return 1
	}

	kind := intent.Classify(transcript)
	_, _ = fmt.Printf("you said: %q\n", transcript)
	_, _ = fmt.Printf("intent: %s\n", kind)

	_ = a.Append(audit.Entry{
		Kind:        "voice.input",
		BlastRadius: 0,
		Outcome:     "success",
		Detail:      kind.String() + ": " + truncateTranscript(transcript, 200),
	})

	switch kind {
	case intent.KindAsk:
		return runAsk(ctx, reg, transcript)
	case intent.KindShip:
		if *repo == "" {
			_, _ = fmt.Println("ship intent detected — re-run with --repo <repo> to dispatch")
			return 0
		}
		return runShip(ctx, *repo, transcript)
	case intent.KindReview:
		if *repo == "" {
			_, _ = fmt.Println("review intent detected — re-run with --repo <repo> to dispatch")
			return 0
		}
		prNum, ok := extractPRNumber(transcript)
		if !ok {
			_, _ = fmt.Fprintln(os.Stderr, "could not parse PR number from transcript")
			return 1
		}
		return runReview(ctx, *repo, prNum)
	case intent.KindStatus:
		s := &dispatcher.Status{
			AuditPath: filepath.Join(stateDir(), "audit.jsonl"),
			Out:       os.Stdout,
			Limit:     20,
		}
		if err := s.Run(); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "leah status: %v\n", err)
			return 1
		}
	}
	return 0
}

// extractPRNumber pulls the first integer token from s. Returns false when
// none is present. Used to bridge "review pr 123" / "review #123" /
// "please review 123" speech into runReview's typed signature.
func extractPRNumber(s string) (int, bool) {
	// Walk runes, accumulate first contiguous digit run.
	var buf strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			buf.WriteRune(r)
			continue
		}
		if buf.Len() > 0 {
			break
		}
	}
	if buf.Len() == 0 {
		return 0, false
	}
	n, err := strconv.Atoi(buf.String())
	if err != nil {
		return 0, false
	}
	return n, true
}

// truncateTranscript returns s clipped to n runes, appending "…" when
// clipped. Kept audit-detail rows short; full transcript stays on stdout
// only. Rune-safe so a multi-byte UTF-8 character cannot be split mid-byte
// (whisper.cpp emits unicode punctuation). Named to disambiguate from
// cmd/leah/backlog.go::truncate.
func truncateTranscript(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
