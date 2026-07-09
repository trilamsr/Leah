package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/trilam/leah/internal/adapters/gcal"
	"github.com/trilam/leah/internal/adapters/gmail"
	"github.com/trilam/leah/internal/brief"
	"github.com/trilam/leah/internal/connect"
	"github.com/trilam/leah/internal/contracts"
	"github.com/trilam/leah/internal/daemonloop"
	"github.com/trilam/leah/internal/feeds"
	commsout "github.com/trilam/leah/internal/comms/out"
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
		data := pullBriefSnapshot(ctx, sd, rc, out)
		// Only push when proactive delivery is opted in — the daemon brief
		// MUST stay silent for operators who run the CLI on demand instead.
		if os.Getenv("LEAH_VOICE_ENABLED") == "1" {
			taskCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			summary := brief.VoiceSummary(data)
			if err := buildBriefNotifier().Notify(taskCtx, "Morning brief", summary); err != nil {
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
func buildBriefNotifier() *commsout.Fanout {
	ns := []contracts.Notifier{commsout.NewDesktop(), commsout.NewVoice()}
	if os.Getenv("LEAH_PUSHOVER_USER") != "" && os.Getenv("LEAH_PUSHOVER_TOKEN") != "" {
		ns = append(ns, commsout.NewPushover())
	}
	return &commsout.Fanout{Notifiers: ns}
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
	if q := newWatchlistQuoter(sd); q != nil {
		o.Watchlist = q
	}
	return o
}

// newWatchlistQuoter builds the AV-backed quoter when the operator has both
// the API key file (provisioned by `leah connect alphavantage`) and a
// watchlist.json with at least one symbol. Either absence → nil, so the brief
// silently omits the section rather than rendering "(unavailable)".
func newWatchlistQuoter(sd string) brief.WatchlistQuoter {
	if len(brief.LoadWatchlistSymbols(filepath.Join(sd, "watchlist.json"))) == 0 {
		return nil
	}
	key, err := loadAVKey(filepath.Join(sd, "secrets", "alphavantage-key.json"))
	if err != nil {
		return nil
	}
	m, err := feeds.NewMarket(feeds.MarketConfig{
		Attestor:   noopFeedsAttestor{},
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
		APIKey:     key,
		BaseURL:    os.Getenv("LEAH_ALPHAVANTAGE_BASE_URL"),
	})
	if err != nil {
		return nil
	}
	return watchlistQuoter{m: m}
}

// watchlistQuoter projects feeds.Quote onto the brief-local shape so brief
// stays free of any feeds import (mirrors the gcalLister projection above).
type watchlistQuoter struct{ m *feeds.Market }

func (w watchlistQuoter) FetchAll(ctx context.Context, symbols []string) ([]brief.WatchlistQuote, error) {
	qs, err := w.m.FetchAll(ctx, symbols)
	if err != nil {
		return nil, err
	}
	out := make([]brief.WatchlistQuote, 0, len(qs))
	for _, q := range qs {
		out = append(out, brief.WatchlistQuote{Symbol: q.Symbol, PercentChange: q.ChangePct, Price: q.Price})
	}
	return out, nil
}

// noopFeedsAttestor auto-approves market.fetch because the daemon brief is
// a scheduled background job — a stdin prompt would block the daily tick.
type noopFeedsAttestor struct{}

func (noopFeedsAttestor) Attest(_ context.Context, _ string) error { return nil }

// loadAVKey mirrors cmd/leah/quote.go's loader but is package-local — the CLI
// version stays self-contained and the daemon can't reach into cmd/leah/.
func loadAVKey(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.Mode().Perm() != 0o600 {
		return "", fmt.Errorf("api key %s has mode %o, want 0600", path, info.Mode().Perm())
	}
	raw, err := os.ReadFile(path) // #nosec G304 — operator-owned state dir
	if err != nil {
		return "", err
	}
	var doc struct {
		APIKey string `json:"api_key"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return "", err
	}
	if doc.APIKey == "" {
		return "", fmt.Errorf("missing api_key")
	}
	return doc.APIKey, nil
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
