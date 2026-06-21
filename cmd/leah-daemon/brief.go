package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/trilam/leah/internal/adapters/discord"
	"github.com/trilam/leah/internal/adapters/gcal"
	"github.com/trilam/leah/internal/adapters/gmail"
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
func buildBriefTask(sd string, rc daemonloop.RegattaClient, out *os.File, discordAdpt *discord.Adapter) daemonloop.WeeklyTask {
	return func(ctx context.Context) {
		data := pullBriefSnapshot(ctx, sd, rc, out)
		// Only push when proactive delivery is opted in — the daemon brief
		// MUST stay silent for operators who run the CLI on demand instead.
		if os.Getenv("LEAH_VOICE_ENABLED") == "1" {
			taskCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			summary := brief.VoiceSummary(data)
			if err := buildBriefNotifier(discordAdpt).Notify(taskCtx, "Morning brief", summary); err != nil {
				_, _ = fmt.Fprintf(out, "leah-daemon: brief push error: %v\n", err)
			}
		}
	}
}

// pullBriefSnapshot is the pull-and-cache half shared by the morning brief and
// the O9 degraded tier — no push, so degraded re-fires never banner/speak.
func pullBriefSnapshot(ctx context.Context, sd string, rc daemonloop.RegattaClient, out *os.File) brief.Data {
	taskCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	now := time.Now()
	data := brief.Gather(taskCtx, now, sd, rc, briefOpts(sd))
	if err := brief.WriteFile(sd, now, brief.Render(data)); err != nil {
		_, _ = fmt.Fprintf(out, "leah-daemon: brief write error: %v\n", err)
	} else {
		_, _ = fmt.Fprintf(out, "leah-daemon: brief written to %s/briefs/%s.md\n", sd, now.Format("2006-01-02"))
	}
	return data
}

func buildDegradedPullTask(sd string, rc daemonloop.RegattaClient, out *os.File) daemonloop.WeeklyTask {
	return func(ctx context.Context) { _ = pullBriefSnapshot(ctx, sd, rc, out) }
}

// buildBriefNotifier fans the brief across every configured push channel;
// each remote joins only when configured so an unset channel stays silent.
func buildBriefNotifier(discordAdpt *discord.Adapter) *notify.Fanout {
	ns := []contracts.Notifier{notify.NewDesktop(), notify.NewVoice()}
	if os.Getenv("LEAH_PUSHOVER_USER") != "" && os.Getenv("LEAH_PUSHOVER_TOKEN") != "" {
		ns = append(ns, notify.NewPushover())
	}
	if d := newDiscordNotifier(discordAdpt); d != nil {
		ns = append(ns, d)
	}
	if w := newWhatsAppNotifier(); w != nil {
		ns = append(ns, w)
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
	wireWorkTools(&o)
	return o
}

// connected reports whether an integration's OAuth token file is present.
func connected(tokenPath string) bool {
	_, err := os.Stat(tokenPath)
	return err == nil
}

// newGmailLister builds the live gmail lister from the stored OAuth token.
// Returns nil — keeping the brief silent — when the Google client creds are
// unset or the token cannot be loaded, so a misconfigured host degrades to
// silent absence rather than an "(unavailable)" banner.
func newGmailLister() brief.GmailLister {
	id, secret := os.Getenv("LEAH_GMAIL_CLIENT_ID"), os.Getenv("LEAH_GMAIL_CLIENT_SECRET")
	if id == "" || secret == "" {
		return nil
	}
	ts, err := connect.LoadRefreshingSource(context.Background(), id, secret, connect.DefaultTokenPath("gmail"))
	if err != nil {
		return nil
	}
	c, err := gmail.New(gmail.Config{
		Attestor:    noopAttestor{},
		TokenSource: ts,
		Transport:   gmail.NewHTTPTransport(nil, ""),
	})
	if err != nil {
		return nil
	}
	return c
}

// newGcalLister mirrors newGmailLister and maps gcal.Event → brief.Event at
// the wire site so the brief package stays free of any adapter import.
func newGcalLister() brief.GcalLister {
	id, secret := os.Getenv("LEAH_GCAL_CLIENT_ID"), os.Getenv("LEAH_GCAL_CLIENT_SECRET")
	if id == "" || secret == "" {
		return nil
	}
	ts, err := connect.LoadRefreshingSource(context.Background(), id, secret, connect.DefaultTokenPath("gcal"))
	if err != nil {
		return nil
	}
	a, err := gcal.New(gcal.Config{
		TokenPath:   connect.DefaultTokenPath("gcal"),
		Attestor:    noopAttestor{},
		TokenSource: ts,
	})
	if err != nil {
		return nil
	}
	return gcalLister{a}
}

// gcalLister adapts gcal.Adapter to brief.GcalLister, projecting each
// gcal.Event onto the two fields Render reads.
type gcalLister struct{ a *gcal.Adapter }

func (g gcalLister) ListToday(ctx context.Context) ([]brief.Event, error) {
	evs, err := g.a.ListToday(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]brief.Event, 0, len(evs))
	for _, e := range evs {
		out = append(out, brief.Event{Start: e.Start, Summary: e.Summary})
	}
	return out, nil
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

// wireDegradedPull arms the O9 degraded tier (LEAH_DEGRADED_PULL=1) only when
// gmail/gcal — the sole adapters with a real lister — is connected, so the
// degraded tick never polls a stub. Pull-only: no socket, no public endpoint.
func wireDegradedPull(loop *daemonloop.Loop, sd string, rc daemonloop.RegattaClient, out *os.File) {
	if os.Getenv("LEAH_DEGRADED_PULL") != "1" {
		return
	}
	if !connected(connect.DefaultTokenPath("gmail")) && !connected(connect.DefaultTokenPath("gcal")) {
		return
	}
	loop.DegradedTracker = filepath.Join(sd, "last-degraded.txt")
	if v := os.Getenv("LEAH_DEGRADED_PULL_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			loop.DegradedInterval = time.Duration(n) * time.Second
		}
	}
	loop.Degraded = []daemonloop.WeeklyTask{buildDegradedPullTask(sd, rc, out)}
	_, _ = fmt.Fprintln(out, "leah-daemon: degraded pull armed (gmail/gcal, no socket)")
}
