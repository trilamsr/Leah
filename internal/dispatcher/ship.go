package dispatcher

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/budget"
	"github.com/trilam/leah/internal/ghclient"
	"github.com/trilam/leah/internal/obs"
	"github.com/trilam/leah/internal/regattaclient"
)

// GHClient is the gh-create-issue surface Ship needs; abstracted so tests
// can capture the issue payload without forking gh.
type GHClient interface {
	CreateIssue(ctx context.Context, args ghclient.CreateIssueArgs) (string, error)
}

// RegattaClient is the regatta-list surface used by the optional watcher.
type RegattaClient interface {
	List(ctx context.Context) ([]regattaclient.Agent, error)
}

// HeartbeatPinger is the per-poll keep-alive hook; failures are non-fatal
// so a missing LEAH_HEALTHCHECK_URL never blocks ship.
type HeartbeatPinger interface {
	Ping(ctx context.Context) error
}

// Notifier emits the terminal-state push on watcher exit (desktop + optional
// Pushover). Failures are non-fatal — operator still sees the URL on stdout.
type Notifier interface {
	Notify(ctx context.Context, title, body string) error
}

// Ship is the BR=3 `leah ship` orchestration: Reasoner draft → gh issue
// create → optional watcher loop until a regatta agent hits terminal state.
type Ship struct {
	Reasoner Reasoner
	GH       GHClient
	Audit    *audit.Logger
	Budget   *budget.Budget
	Out      io.Writer
	Repo     string
	Title    string
	TmpDir   string

	// Context is an optional markdown block (PR / issue / shell-history
	// fetched by the caller via FetchPRContext / FetchIssueContext /
	// FetchThreadContext + ComposeContext) prepended to the Reasoner draft
	// prompt so the model sees the referenced artifacts before drafting.
	Context string

	// Watcher (optional; Watch=false → ship-and-exit)
	Watch     bool
	Regatta   RegattaClient
	Heartbeat HeartbeatPinger
	Notify    Notifier
	PollEvery time.Duration
	MaxPolls  int

	// LastURL is the issue URL from the most recent successful Run; consumed
	// by SelfBuild to surface the URL on its BR=4 self-build audit row so
	// the H3 attestation gate can correlate dispatches to PRs.
	LastURL string
}

// Run drafts the issue body, files via gh, and (when Watch=true) polls
// regatta until a terminal agent transition or MaxPolls. Audit row is
// written immediately as "pending" — watcher does not retroactively update.
func (s *Ship) Run(ctx context.Context, intent string) error {
	ulidVal, err := ulid.New(ulid.Now(), ulid.Monotonic(rand.Reader, 0))
	if err != nil {
		return fmt.Errorf("ulid: %w", err)
	}
	actionID := ulidVal.String()
	lg := obs.LoggerFromCtx(ctx).With("package", "dispatcher", "action_id", actionID)

	lg.Info("dispatcher.draft.start")
	prompt := s.Context + "Intent:\n" + intent + "\n\nDraft the regatta issue body per the template you were given."
	bodyDraft, err := s.Reasoner.Ask(ctx, prompt)
	if err != nil {
		lg.Error("dispatcher.draft.error", "err", err.Error())
		s.auditFail("draft body: "+err.Error(), intent)
		return err
	}

	cleanDraft := strings.ReplaceAll(bodyDraft, "-->", "-- >")
	body := cleanDraft + "\n\n<!-- leah-dispatched: " + actionID + " -->\n<!-- intent-source: terminal -->\n"

	bodyFile := filepath.Join(s.TmpDir, "leah-ship-"+actionID+".md")
	if err := os.WriteFile(bodyFile, []byte(body), 0o600); err != nil {
		lg.Error("dispatcher.write.error", "path", bodyFile, "err", err.Error())
		s.auditFail("write body file: "+err.Error(), intent)
		return err
	}
	lg.Info("dispatcher.write.complete", "path", bodyFile, "body_bytes", len(body))

	title := s.Title
	if title == "" {
		title = deriveTitle(intent)
	}

	url, err := s.GH.CreateIssue(ctx, ghclient.CreateIssueArgs{
		Repo:     s.Repo,
		Title:    title,
		BodyFile: bodyFile,
		Labels:   []string{"ready-for-agent"},
	})
	if err != nil {
		lg.Error("dispatcher.issue.error", "repo", s.Repo, "err", err.Error())
		s.auditFail("gh issue create: "+err.Error(), intent)
		return err
	}
	lg.Info("dispatcher.issue.created", "url", url, "title", title)

	if err := s.Audit.Append(audit.Entry{
		Kind:        "ship",
		ArgsHash:    argsHash(intent),
		BlastRadius: 3,
		Outcome:     "pending",
		CostDollars: s.Budget.Spent(),
		Detail:      url,
	}); err != nil {
		_, _ = fmt.Fprintf(s.Out, "warning: audit append failed: %v\n", err)
	}

	s.LastURL = url
	_, _ = fmt.Fprintln(s.Out, url)

	if s.Watch {
		lg.Info("dispatcher.watch.start", "poll_every", s.PollEvery.String(), "max_polls", s.MaxPolls)
		s.watch(ctx)
	}
	return nil
}

func (s *Ship) watch(ctx context.Context) {
	lg := obs.LoggerFromCtx(ctx).With("package", "dispatcher")
	polls := 0
	for {
		if s.MaxPolls > 0 && polls >= s.MaxPolls {
			return
		}
		polls++
		if s.Heartbeat != nil {
			_ = s.Heartbeat.Ping(ctx)
		}
		if s.Regatta != nil {
			agents, err := s.Regatta.List(ctx)
			if err != nil {
				// Mirror to obs so daemon-mode watchers (no attached
				// terminal) do not lose the signal (audit M5).
				lg.Warn("dispatcher.watch.regatta_list_error", "err", err.Error())
				_, _ = fmt.Fprintf(s.Out, "regatta list error: %v\n", err)
			}
			for _, a := range agents {
				if a.State == "merged" || a.State == "escalated" || a.State == "failed" {
					if s.Notify != nil {
						_ = s.Notify.Notify(ctx, "Leah",
							fmt.Sprintf("agent %s: %s (PR #%d)", a.ID, a.State, a.PR))
					}
					return
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(s.PollEvery):
		}
	}
}

func (s *Ship) auditFail(detail, intent string) {
	_ = s.Audit.Append(audit.Entry{
		Kind:        "ship",
		ArgsHash:    argsHash(intent),
		BlastRadius: 3,
		Outcome:     "failed",
		CostDollars: s.Budget.Spent(),
		Detail:      detail,
	})
}

func deriveTitle(intent string) string {
	t := strings.TrimSpace(intent)
	// Pass-through if operator already prefixed
	for _, p := range []string{"[FEAT]", "[FIX]", "[DOCS]", "[CHORE]", "[REFACTOR]", "[CI]"} {
		if strings.HasPrefix(t, p) {
			if len(t) > 60 {
				t = t[:57] + "..."
			}
			return t
		}
	}
	prefix := classifyIntentPrefix(t)
	if len(t) > 60-len(prefix)-1 {
		t = t[:57-len(prefix)-1] + "..."
	}
	return prefix + " " + t
}

func classifyIntentPrefix(s string) string {
	low := strings.ToLower(strings.TrimSpace(s))
	switch {
	case strings.HasPrefix(low, "fix"), strings.HasPrefix(low, "patch"), strings.HasPrefix(low, "repair"):
		return "[FIX]"
	case strings.HasPrefix(low, "doc"), strings.HasPrefix(low, "document"):
		return "[DOCS]"
	case strings.HasPrefix(low, "refactor"):
		return "[REFACTOR]"
	case strings.HasPrefix(low, "chore"):
		return "[CHORE]"
	default:
		return "[FEAT]"
	}
}
