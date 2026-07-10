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
	"github.com/trilam/leah/internal/contracts"
	"github.com/trilam/leah/internal/ghclient"
	"github.com/trilam/leah/internal/obs"
	"github.com/trilam/leah/internal/regattaclient"
)

// GHClient is the gh surface Ship + SelfBuild need; abstracted so tests can
// capture the issue payload without forking gh. EnsureLabel is the
// idempotent label-create used by the Defect-1 retry path when CreateIssue
// fails because the `ready-for-agent` label is missing on the target repo.
type GHClient interface {
	CreateIssue(ctx context.Context, args ghclient.CreateIssueArgs) (string, error)
	EnsureLabel(ctx context.Context, repo, label string) error
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

// Ship is the BR=3 `leah ship` orchestration: Reasoner draft → gh issue
// create → optional watcher loop until a regatta agent hits terminal state.
//
// When invoked by SelfBuild, the Reasoner field MUST be a prebakedReasoner
// (selfbuild.go): the spec has already been drafted under the self-build
// system prompt and the second Reasoner.Ask call here returns it verbatim
// rather than re-prompting under the regatta-issue ship template.
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
	Notify    contracts.Notifier
	PollEvery time.Duration
	MaxPolls  int

	// LastURL is the issue URL from the most recent successful Run; consumed
	// by SelfBuild to surface the URL on its BR=4 self-build audit row so
	// the H3 attestation gate can correlate dispatches to PRs.
	LastURL string

	// OnOutcome fires once per Run with the terminal outcome ("pending" on
	// successful file, "failed" on auditFail). Wired by main.go to feed
	// leah_dispatcher_ship_total{outcome}. nil-safe.
	OnOutcome func(outcome string)
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
	lg := telemetry.LoggerFromCtx(ctx).With("package", "dispatcher", "action_id", actionID)

	lg.Info("dispatcher.draft.start")
	prompt := s.Context + "Intent:\n" + intent + "\n\nDraft the regatta issue body per the template you were given."
	bodyDraft, err := s.Reasoner.Ask(ctx, prompt)
	if err != nil {
		lg.Error("dispatcher.draft.error", "err", err.Error())
		s.auditFail("draft body: "+err.Error(), intent)
		return err
	}

	// Guard the leah-dispatched sentinel: a stray --> in the draft would close it mid-record.
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

	createArgs := ghclient.CreateIssueArgs{
		Repo:     s.Repo,
		Title:    title,
		BodyFile: bodyFile,
		Labels:   []string{"ready-for-agent"},
	}
	url, err := s.GH.CreateIssue(ctx, createArgs)
	if err != nil && isMissingLabelErr(err) {
		// Defect-1 retry: gh failed because `ready-for-agent` does not
		// exist on the target repo. Auto-create the label and retry once.
		// Original error is preserved if the retry also fails so the
		// operator sees the actionable root cause.
		lg.Warn("dispatcher.issue.label_missing_retry", "repo", s.Repo, "err", err.Error())
		if ensureErr := s.GH.EnsureLabel(ctx, s.Repo, "ready-for-agent"); ensureErr != nil {
			lg.Error("dispatcher.issue.ensure_label_error", "repo", s.Repo, "err", ensureErr.Error())
			s.auditFail("gh label create: "+ensureErr.Error(), intent)
			return ensureErr
		}
		var retryErr error
		url, retryErr = s.GH.CreateIssue(ctx, createArgs)
		if retryErr != nil {
			lg.Error("dispatcher.issue.error", "repo", s.Repo, "err", err.Error(), "retry_err", retryErr.Error())
			s.auditFail("gh issue create: "+err.Error(), intent)
			return err
		}
		err = nil
	}
	if err != nil {
		lg.Error("dispatcher.issue.error", "repo", s.Repo, "err", err.Error())
		s.auditFail("gh issue create: "+err.Error(), intent)
		return err
	}
	lg.Info("dispatcher.issue.created", "url", url, "title", title)
	telemetry.Publish(telemetry.Event{
		TS:      time.Now().UTC(),
		Kind:    "dispatch.ship",
		Actor:   "dispatcher",
		Outcome: "pending",
		Target:  title,
		Detail:  url,
	})

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
	if s.OnOutcome != nil {
		s.OnOutcome("pending")
	}
	_, _ = fmt.Fprintln(s.Out, url)

	if s.Watch {
		lg.Info("dispatcher.watch.start", "poll_every", s.PollEvery.String(), "max_polls", s.MaxPolls)
		_, _ = s.watch(ctx)
	}
	return nil
}

// watch polls regatta until a terminal agent transition, MaxPolls, or
// ctx.Done. Returns the terminal-state classifier — one of "merged" /
// "escalated" / "failed" (regatta agent transitioned), "timeout" (MaxPolls
// exhausted), or "cancelled" (ctx.Done) — and the terminal agent's PR (0
// for non-transition exits). SelfBuild records the PR on its
// kind=self-build.outcome row so the daemon can bind a later merge
// transition back to this dispatch's ArgsHash.
func (s *Ship) watch(ctx context.Context) (string, int) {
	lg := telemetry.LoggerFromCtx(ctx).With("package", "dispatcher")
	polls := 0
	for {
		if s.MaxPolls > 0 && polls >= s.MaxPolls {
			return "timeout", 0
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
					if a.State == "merged" {
						telemetry.Publish(telemetry.Event{
							TS:      time.Now().UTC(),
							Kind:    "dispatch.merge",
							Actor:   "dispatcher",
							Outcome: "ok",
							Target:  a.ID,
							Detail:  fmt.Sprintf("PR #%d", a.PR),
						})
					}
					if s.Notify != nil {
						_ = s.Notify.Notify(ctx, "Leah",
							fmt.Sprintf("agent %s: %s (PR #%d)", a.ID, a.State, a.PR))
					}
					return a.State, a.PR
				}
			}
		}
		select {
		case <-ctx.Done():
			return "cancelled", 0
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
	if s.OnOutcome != nil {
		s.OnOutcome("failed")
	}
}

// isMissingLabelErr decides whether a CreateIssue failure looks like
// "the --label we passed does not exist on the repo". Pattern-matches gh's
// known stderr surfaces:
//   - `could not add label: 'ready-for-agent' not found`
//   - `label not found`
//   - REST/GraphQL `validation failed` (404 from labels mutation)
//
// Deliberately does NOT match bare `not found` — that would swallow auth /
// repo-not-found errors and mask the actual root cause. The retry only fires
// when the message names a label or labels API failure.
func isMissingLabelErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "'ready-for-agent' not found"),
		strings.Contains(msg, "label not found"),
		strings.Contains(msg, "could not add label"),
		strings.Contains(msg, "validation failed"):
		return true
	}
	return false
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
