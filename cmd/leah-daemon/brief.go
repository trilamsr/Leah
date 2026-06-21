package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/trilam/leah/internal/brief"
	"github.com/trilam/leah/internal/connect"
	"github.com/trilam/leah/internal/contracts"
	"github.com/trilam/leah/internal/daemonloop"
	"github.com/trilam/leah/internal/notify"
)

// buildBriefTask returns the morning-brief task (appended to weekly tasks
// by default; promoted to the daily list when LEAH_BRIEF_DAILY=1). Composes
// the same brief the CLI prints, writes it to ~/.leah-state/briefs/
// YYYY-MM-DD.md (idempotent per-day overwrite — daily re-fire on the same
// calendar day overwrites the prior file rather than appending), and —
// when LEAH_VOICE_ENABLED=1 — speaks the 1-sentence summary + pushes a
// desktop banner. 30s per-task ctx budget mirrors the cmd/leah brief CLI
// so a hung regattaclient.List call cannot block the weekly goroutine
// until daemon shutdown. Soft-fails per surface: TTS error never gates
// the file write, file-write error never gates voice/desktop.
func buildBriefTask(sd string, rc daemonloop.RegattaClient, out *os.File) daemonloop.WeeklyTask {
	return func(ctx context.Context) {
		taskCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		now := time.Now()
		data := brief.Gather(taskCtx, now, sd, rc, briefOpts(sd))
		body := brief.Render(data)
		if err := brief.WriteFile(sd, now, body); err != nil {
			_, _ = fmt.Fprintf(out, "leah-daemon: brief write error: %v\n", err)
		} else {
			_, _ = fmt.Fprintf(out, "leah-daemon: brief written to %s/briefs/%s.md\n", sd, now.Format("2006-01-02"))
		}
		// Only push when proactive delivery is opted in — the daemon brief
		// MUST stay silent for operators who run the CLI on demand instead.
		if os.Getenv("LEAH_VOICE_ENABLED") == "1" {
			summary := brief.VoiceSummary(data)
			if err := buildBriefNotifier().Notify(taskCtx, "Morning brief", summary); err != nil {
				_, _ = fmt.Fprintf(out, "leah-daemon: brief push error: %v\n", err)
			}
		}
	}
}

// buildBriefNotifier fans the brief summary across every configured push
// channel. Desktop + voice are the opt-in pair; pushover joins only when its
// creds are set so an unconfigured phone push stays silent rather than
// erroring on every fire. Slack/discord/whatsapp have no contracts.Notifier
// yet — wiring one is the next-wave adapter work, not this push path.
func buildBriefNotifier() *notify.Fanout {
	ns := []contracts.Notifier{notify.NewDesktop(), notify.NewVoice()}
	if os.Getenv("LEAH_PUSHOVER_USER") != "" && os.Getenv("LEAH_PUSHOVER_TOKEN") != "" {
		ns = append(ns, notify.NewPushover())
	}
	return &notify.Fanout{Notifiers: ns}
}

// briefOpts wires gmail + gcal into the live daemon brief, gated on the
// operator having connected the integration (its OAuth token file present).
// An absent token yields a nil lister so Gather omits the section — unconfigured
// is silent absence, not "(unavailable)". Each lister is built only when its
// token is present so a never-connected integration stays silent.
func briefOpts(sd string) brief.GatherOpts {
	var o brief.GatherOpts
	if connected(connect.DefaultTokenPath("gmail")) {
		if c := newGmailLister(); c != nil {
			o.Gmail = c
		}
	}
	if connected(connect.DefaultTokenPath("gcal")) {
		if c := newGcalLister(); c != nil {
			o.Gcal = c
		}
	}
	return o
}

// connected reports whether an integration's OAuth token file is present.
func connected(tokenPath string) bool {
	_, err := os.Stat(tokenPath)
	return err == nil
}

// newGmailLister returns a connected gmail lister, or nil while the adapter
// lacks a production gmail.Transport (it ships only a test seam) — a nil
// keeps the brief silent rather than branding the inbox "(unavailable)".
func newGmailLister() brief.GmailLister { return nil }

// newGcalLister mirrors newGmailLister: nil until the adapter ships a
// production calendarService.
func newGcalLister() brief.GcalLister { return nil }

// wireBriefSchedule attaches briefTask to either the daily or weekly slot on
// loop based on LEAH_BRIEF_DAILY. LEAH_BRIEF_DAILY=1 promotes the brief to
// the independent daily list so the brief lands every morning instead of
// once a week. LEAH_BRIEF_HOUR (default 8) gates the daily fire — the brief
// should not wake the operator at 03:00 if the daemon restarts overnight.
func wireBriefSchedule(loop *daemonloop.Loop, sd string, briefTask daemonloop.WeeklyTask, out *os.File) {
	if os.Getenv("LEAH_BRIEF_DAILY") != "1" {
		loop.Weekly = append(loop.Weekly, briefTask)
		return
	}
	loop.DailyTracker = filepath.Join(sd, "last-daily.txt")
	loop.DailyHour = 8
	if v := os.Getenv("LEAH_BRIEF_HOUR"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 && n <= 23 {
			loop.DailyHour = n
		}
	}
	loop.Daily = []daemonloop.WeeklyTask{briefTask}
	_, _ = fmt.Fprintf(out, "leah-daemon: brief = daily @ hour %d\n", loop.DailyHour)
}
