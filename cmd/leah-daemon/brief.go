package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/trilam/leah/internal/brief"
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
		data := brief.Gather(taskCtx, now, sd, rc)
		body := brief.Render(data)
		if err := brief.WriteFile(sd, now, body); err != nil {
			_, _ = fmt.Fprintf(out, "leah-daemon: brief write error: %v\n", err)
		} else {
			_, _ = fmt.Fprintf(out, "leah-daemon: brief written to %s/briefs/%s.md\n", sd, now.Format("2006-01-02"))
		}
		// Only speak/push when voice is opted in — the daemon brief MUST
		// stay silent for operators who run the CLI on demand instead.
		if os.Getenv("LEAH_VOICE_ENABLED") == "1" {
			summary := brief.VoiceSummary(data)
			if v := notify.NewVoice(); v != nil {
				if err := v.Notify(taskCtx, "Morning brief", summary); err != nil {
					_, _ = fmt.Fprintf(out, "leah-daemon: brief voice error: %v\n", err)
				}
			}
			if d := notify.NewDesktop(); d != nil {
				if err := d.Notify(taskCtx, "Leah: morning brief", summary); err != nil {
					_, _ = fmt.Fprintf(out, "leah-daemon: brief desktop error: %v\n", err)
				}
			}
		}
	}
}

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
