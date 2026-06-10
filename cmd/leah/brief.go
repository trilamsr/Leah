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

// runBrief implements `leah brief [--voice] [--silent]`. Composes a terse
// morning brief from existing data sources (audit log + memory.db +
// regatta CLI + bug-fix-candidates.md) — no LLM call by default. All
// composition logic lives in internal/brief so the daemon's daily-task
// can reuse it without re-implementing the CLI wrapper.
func runBrief(parent context.Context, args []string) {
	if shouldShowHelp(args) {
		_, _ = fmt.Fprintln(os.Stderr, "usage: leah brief [--voice] [--silent]")
		return
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
			os.Exit(2)
		}
	}

	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()

	now := time.Now()
	data := brief.Gather(ctx, now, stateDir(), regattaclient.New())
	body := brief.Render(data)

	// --silent precedence: file only, no stdout, no voice (cron/launchd path
	// where TTS would be a regression). --voice on its own → stdout + speak.
	if silentMode {
		if err := brief.WriteFile(stateDir(), now, body); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "leah brief: write file: %v\n", err)
			os.Exit(1)
		}
		return
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
}
