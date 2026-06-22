package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/trilam/leah/internal/brief"
	"github.com/trilam/leah/internal/notify"
	"github.com/trilam/leah/internal/regattaclient"
)

// runBrief implements `leah brief [--voice] [--silent] [--since=<RFC3339>]`. Composes a terse
// morning brief from existing data sources (audit log + memory.db +
// regatta CLI + bug-fix-candidates.md) — no LLM call by default. All
// composition logic lives in internal/brief so the daemon's daily-task
// can reuse it without re-implementing the CLI wrapper.
func runBrief(parent context.Context, args []string) int {
	if shouldShowHelp(args) {
		_, _ = fmt.Fprintln(os.Stderr, "usage: leah brief [--voice] [--silent] [--since=<RFC3339>]")
		return 0
	}
	anchor, args, err := ParseSince(args)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah brief: %v\nusage: leah brief [--voice] [--silent] [--since=<RFC3339>]\n", err)
		return 2
	}
	voiceMode := false
	silentMode := false
	for _, a := range args {
		switch a {
		case "--voice":
			voiceMode = true
		case "--silent":
			silentMode = true
		default:
			_, _ = fmt.Fprintf(os.Stderr, "leah brief: unknown flag %q\n", a)
			return 2
		}
	}

	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()

	// --since shifts the brief's wall-clock anchor backward so "yesterday"
	// recap and week-to-date windows are computed relative to the cursor,
	// not real-now. Lets operators replay a brief as-of an earlier moment
	// without changing internal/brief.Gather's signature.
	now := time.Now()
	if !anchor.IsZero() {
		now = anchor
	}
	data := brief.Gather(ctx, now, stateDir(), regattaclient.New())
	body := brief.Render(data)

	// --silent precedence: file only, no stdout, no voice (cron/launchd path
	// where TTS would be a regression). --voice on its own → stdout + speak.
	if silentMode {
		if err := brief.WriteFile(stateDir(), now, body); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "leah brief: write file: %v\n", err)
			return 1
		}
		return 0
	}

	_, _ = fmt.Print(body)

	if voiceMode {
		v := notify.NewVoice()
		// Speak a 1-line summary, not the full markdown — TTS on long
		// markdown is painful and the operator already sees stdout.
		summary := brief.VoiceSummary(data)
		if err := v.Notify(ctx, "Morning brief", summary); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "leah brief: voice: %v\n", err)
			// Not fatal — text already printed.
		}
	}
	return 0
}
