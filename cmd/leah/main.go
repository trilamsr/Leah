package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/budget"
	"github.com/trilam/leah/internal/budget/monthly"
	"github.com/trilam/leah/internal/dispatcher"
	"github.com/trilam/leah/internal/ghclient"
	commsout "github.com/trilam/leah/internal/comms/out"
	"github.com/trilam/leah/internal/obs"
	"github.com/trilam/leah/internal/onboarding"
	"github.com/trilam/leah/internal/persona"
	"github.com/trilam/leah/internal/reasoner"
	"github.com/trilam/leah/internal/regattaclient"
	"github.com/trilam/leah/internal/reviewer"
	"github.com/trilam/leah/internal/watchdog"
)

const version = "0.0.1-mvp5"

func main() { os.Exit(run()) }

// run owns the defer chain that os.Exit would otherwise skip. Subcommands
// MUST return int (no os.Exit) so writeInterruptedAudit fires on SIGINT
// (review #55: previous version's os.Exit inside subcommands bypassed
// the audit-flush this PR is supposed to deliver).
func run() int {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	sd, err := stateDirE()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah: %v\n", err)
		return 1
	}
	auditPath := filepath.Join(sd, "audit.jsonl")
	return runAndFlush(ctx, auditPath, os.Args[1:])
}

// runAndFlush is run() minus the signal wiring — extracted so tests can
// drive the defer-flush path with a pre-canceled ctx without forking.
func runAndFlush(ctx context.Context, auditPath string, args []string) int {
	defer writeInterruptedAudit(ctx, auditPath)
	reg := obs.NewRegistry()
	defer snapshotCLIMetrics(reg)
	return runCommand(ctx, reg, args)
}

// snapshotCLIMetrics writes the CLI registry to <stateDir>/metrics/cli-latest.json.
// Best-effort — a snapshot failure on the exit path can't surface as an error
// because the program is already done.
func snapshotCLIMetrics(reg *obs.Registry) {
	if reg == nil {
		return
	}
	sd, err := stateDirE()
	if err != nil {
		return
	}
	_ = reg.Snapshot(filepath.Join(sd, "metrics", "cli-latest.json"))
}

// runCommand dispatches argv (without the program name) under a shared ctx.
// Extracted from main() so tests can drive the dispatcher with an arbitrary
// (possibly canceled) ctx without spawning a subprocess.
//
// The reg argument owns the per-invocation metric registry. The wrapped
// `stdout` below is threaded only into subcommands whose Out writer carries
// pre-LLM bytes (status, connect, whoami, version, …). LLM-emitting paths
// (ask, ship) deliberately receive raw os.Stdout — wrapping them would let
// the first model token trip the leah_cli_dispatch_to_first_byte_seconds
// histogram and violate the "excl. LLM" contract. Result: ask/ship contribute
// zero observations, which is the correct null for paths with no non-LLM
// first byte to measure. A nil reg disables instrumentation (test paths).
func runCommand(ctx context.Context, reg *obs.Registry, args []string) int {
	if len(args) < 1 {
		usage()
		return 2
	}

	cmd := args[0]
	rest := args[1:]
	fbt := dispatcher.NewFirstByteTimer(reg, cmd)
	stdout := fbt.Wrap(os.Stdout)
	// Progress fires once for commands users expect to take >100ms; emitted
	// inline right before dispatch so the histogram captures argv-parse +
	// flag-set overhead, which is the latency band A7 tracks.
	if longRunningCLICommand(cmd) {
		dispatcher.NewProgressTimer(reg, cmd).Emit()
	}
	switch cmd {
	case "version", "-v", "--version":
		_, _ = fmt.Fprintln(stdout, version)
		return 0
	case "ask":
		if len(rest) < 1 || shouldShowHelp(rest) {
			_, _ = fmt.Fprintln(os.Stderr, "usage: leah ask \"<query>\"")
			if len(rest) >= 1 && shouldShowHelp(rest) {
				return 0
			}
			return 2
		}
		return runAsk(ctx, reg, rest[0])
	case "ship":
		return runShipArgs(ctx, rest)
	case "call":
		return runCall(ctx, rest)
	case "review":
		if shouldShowHelp(rest) {
			_, _ = fmt.Fprintln(os.Stderr, "usage: leah review <repo> <pr#>")
			return 0
		}
		if len(rest) < 2 {
			_, _ = fmt.Fprintln(os.Stderr, "usage: leah review <repo> <pr#>")
			return 2
		}
		var prNum int
		if _, err := fmt.Sscanf(rest[1], "%d", &prNum); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, "pr# must be an integer")
			return 2
		}
		return runReview(ctx, rest[0], prNum)
	case "status":
		if shouldShowHelp(rest) {
			_, _ = fmt.Fprintln(os.Stderr, "usage: leah status [--json]")
			return 0
		}
		jsonMode := false
		for _, a := range rest {
			if a == "--json" {
				jsonMode = true
			}
		}
		s := &dispatcher.Status{
			AuditPath: filepath.Join(stateDir(), "audit.jsonl"),
			Out:       stdout,
			Limit:     20,
			JSON:      jsonMode,
		}
		if err := s.Run(); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "leah status: %v\n", err)
			return 1
		}
	case "contact":
		return runContact(rest)
	case "project":
		return runProject(rest)
	case "decision":
		return runDecision(rest)
	case "ctx":
		return runCtx(rest)
	case "workspace":
		return runWorkspace(rest)
	case "mistake":
		return runMistake(rest)
	case "retro":
		return runRetro(rest)
	case "patterns":
		return runPatterns(rest)
	case "suggest":
		return runSuggest(ctx, rest)
	case "backlog":
		return runBacklog(ctx, rest)
	case "recall":
		return runRecall(ctx, rest)
	case "self-build":
		if shouldShowHelp(rest) {
			_, _ = fmt.Fprintln(os.Stderr, "usage: leah self-build \"<intent>\"")
			return 0
		}
		if len(rest) < 1 {
			_, _ = fmt.Fprintln(os.Stderr, "usage: leah self-build \"<intent>\"")
			return 2
		}
		return runSelfBuild(ctx, rest[0])
	case "self-build-status":
		return runSelfBuildStatus(rest, stdout)
	case "cost":
		return runCost(rest)
	case "brief":
		return runBrief(ctx, rest)
	case "listen":
		return runListen(ctx, reg, rest)
	case "backup":
		return runBackup(ctx, rest)
	case "init":
		return runInit(ctx, rest, stdout, os.Stdin)
	case "connect":
		return runConnect(ctx, rest, stdout)
	case "disconnect":
		return runDisconnect(ctx, rest, stdout)
	case "forget":
		return runForget(ctx, rest, stdout)
	case "purge":
		return runPurge(ctx, rest, stdout)
	case "news":
		return runNews(ctx, rest, stdout)
	case "oncall":
		return runOncallDefault(rest)
	case "paper":
		return runPaper(ctx, rest, stdout, os.Stderr)
	case "quote":
		return runQuote(ctx, rest, stdout)
	case "watch":
		return runWatch(rest, stdout, os.Stderr)
	case "whoami":
		return runWhoami(ctx, rest, stdout)
	case "earnings":
		return runEarnings(ctx, rest, stdout)
	case "export":
		return runExport(ctx, rest, os.Stderr)
	case "import":
		return runImport(ctx, rest, os.Stderr)
	case "inbound":
		return runInbound(ctx, rest, stdout)
	case "self-upgrade":
		return runSelfUpgrade(ctx, rest, stdout, nil)
	case "pr-state":
		return runPRState(ctx, nil, rest, stdout)
	case "review-queue":
		return runReviewQueue(ctx, rest, stdout)
	case "trip":
		return runTrip(ctx, rest, stdout)
	case "open":
		return runOpen(ctx, rest, stdout)
	case "find":
		return runFind(ctx, rest, stdout)
	case "strategist":
		return runStrategist(ctx, rest, stdout)
	case "keychain":
		return runKeychain(ctx, rest, os.Stdin, stdout)
	default:
		usage()
		return 2
	}
	return 0
}

// longRunningCLICommand is the gate for leah_cli_dispatch_to_first_progress_seconds.
// Listed here are the subcommands a user expects to take long enough that the
// shell prompt visibly stalls without an "I'm working" beat; short pure-local
// commands (status, whoami, version, …) are excluded so the histogram isn't
// diluted by latencies whose SLA is already tracked by first-byte.
func longRunningCLICommand(cmd string) bool {
	switch cmd {
	case "ask", "ship", "brief", "self-build", "review",
		"suggest", "backlog", "recall", "news", "self-upgrade", "pr-state":
		return true
	}
	return false
}

// writeInterruptedAudit appends one Outcome="interrupted" row when ctx was
// canceled (SIGINT/SIGTERM). No-op on clean exit. Errors are dropped on
// purpose — the program is already tearing down and stderr noise from a
// best-effort flush helps nobody.
func writeInterruptedAudit(ctx context.Context, auditPath string) {
	if ctx.Err() == nil {
		return
	}
	a := &audit.Logger{Path: auditPath, DefaultWorkspace: activeWorkspace}
	_ = a.Append(audit.Entry{
		Kind:        "cli.interrupted",
		BlastRadius: 0,
		Outcome:     "interrupted",
		Detail:      ctx.Err().Error(),
	})
}

// openCostBreaker opens the per-operator monthly cost ledger and wraps it
// in a reasoner.Breaker. Non-fatal: a nil breaker is acceptable — the
// Router will skip the gate and the CLI invocation will run uncapped.
func openCostBreaker() reasoner.Breaker {
	cap := 50.0
	if v := os.Getenv("LEAH_COST_MONTH_CAP"); v != "" {
		if parsed, err := strconv.ParseFloat(v, 64); err == nil && parsed > 0 {
			cap = parsed
		}
	}
	store, err := monthly.OpenAt(filepath.Join(stateDir(), "cost-month.json"), cap, time.Now())
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah: monthly-cost open non-fatal: %v\n", err)
		return nil
	}
	return monthly.NewBreaker(store)
}

// askStreamer is the surface runAskWith depends on — *reasoner.Reasoner
// satisfies it; tests wire a scripted fake. Kept narrow so the test fake
// doesn't drag in budget/audit setup.
type askStreamer interface {
	AskStream(ctx context.Context, user string) (<-chan string, error)
}

func runAsk(ctx context.Context, reg *obs.Registry, query string) int {
	systemPrompt, err := os.ReadFile(filepath.Join(promptDir(), "system.md"))
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "read system prompt: %v\n", err)
		return 1
	}

	b := budget.New()
	br := openCostBreaker()
	client, err := reasoner.NewRoutedClient("reasoner", br, b, nil)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah ask: %v\n", err)
		return 1
	}
	r := &reasoner.Reasoner{Client: client, Budget: b, SystemPrompt: string(systemPrompt), PersonaPrefix: personaPrefixForActive()}

	a := &audit.Logger{Path: filepath.Join(stateDir(), "audit.jsonl"), DefaultWorkspace: activeWorkspace}
	code := runAskWith(ctx, r, os.Stdout, a, b, query)
	if code == 0 {
		// A9 SLA close-out: filesystem-sealed so a second `leah ask` (new
		// process, fresh registry) cannot record a near-zero sample.
		onboarding.RecordFirstReplyIfNotYetRecorded(stateDir(), reg, time.Now())
	}
	return code
}

// runAskWith drains AskStream's delta channel straight into out — each
// delta becomes its own Write so the firstByteWriter wrapping os.Stdout
// (A6) sees the first model token at first-delta time, not full-reply
// time. Audit row is written on every exit path (success + any write or
// stream error) to preserve the charged-Reasoner-call invariant the old
// dispatcher.Ask.Run held.
func runAskWith(ctx context.Context, s askStreamer, out io.Writer, a *audit.Logger, b *budget.Budget, query string) int {
	entry := audit.Entry{Kind: "ask", ArgsHash: askArgsHash(query), BlastRadius: 0}
	finish := func(code int, detail string) int {
		entry.CostDollars = b.Spent()
		if rich, ok := any(s).(dispatcher.LLMDimReporter); ok {
			info := rich.LastCallInfo()
			entry.Model = info.Model
			entry.PromptSHA = info.PromptSHA
			entry.InputTokens = info.InputTokens
			entry.OutputTokens = info.OutputTokens
			entry.LatencyMS = info.LatencyMS
			entry.EgressBytes = info.EgressBytes
			entry.CacheHit = info.CacheHit
		}
		if code == 0 {
			entry.Outcome = "success"
		} else {
			entry.Outcome = "failed"
			entry.Detail = detail
		}
		_ = a.Append(entry)
		return code
	}
	write := func(p string) error {
		_, werr := io.WriteString(out, p)
		return werr
	}

	ch, err := s.AskStream(ctx, query)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah ask: %v\n", err)
		return finish(1, err.Error())
	}
	for delta := range ch {
		if werr := write(delta); werr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "leah ask: %v\n", werr)
			return finish(1, werr.Error())
		}
	}
	if werr := write("\n"); werr != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah ask: %v\n", werr)
		return finish(1, werr.Error())
	}
	return finish(0, "")
}

func askArgsHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:8])
}

// runShipArgs parses the `leah ship` flag set + dispatches. Supports:
//
//	leah ship [--from-pr N] [--from-issue N] [--from-thread Wc|Wm] <repo> "<intent>"
//
// Each --from-* flag fetches a context block (via gh / shell history); blocks
// are composed in PR → issue → thread order and prepended to the Reasoner
// draft prompt so the model sees the referenced artifacts before drafting.
func runShipArgs(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("ship", flag.ContinueOnError)
	fromPR := fs.Int("from-pr", 0, "prepend gh pr view + diff for PR #N from the same repo")
	fromIssue := fs.Int("from-issue", 0, "prepend gh issue view + comments for issue #N from the same repo")
	fromThread := fs.String("from-thread", "", "prepend last-N shell-history entries (e.g. 100c or 30m)")
	imsgTo := fs.String("imessage", "", "iMessage \"<body>\" to <to> (attestation-gated)")
	fs.Usage = func() {
		_, _ = fmt.Fprintln(os.Stderr, "usage: leah ship [--from-pr N] [--from-issue N] [--from-thread Wc|Wm] <repo> \"<intent>\"")
		_, _ = fmt.Fprintln(os.Stderr, "       leah ship --imessage <to> \"<body>\"")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *imsgTo != "" {
		if fs.NArg() < 1 {
			fs.Usage()
			return 2
		}
		return runShipIMessageWith(ctx, newConnectAttestor(), nativeExec{}, *imsgTo, fs.Arg(0))
	}

	if fs.NArg() < 2 {
		fs.Usage()
		return 2
	}
	repo := fs.Arg(0)
	intent := fs.Arg(1)

	// Build optional context block(s). Each fetcher is soft-fail: warn,
	// continue without that block — operator dispatching shouldn't be blocked
	// by a stale PR number or a missing zsh history file.
	exec := ghclient.ShellExec{}
	var prCtx, issueCtx, threadCtx string
	if *fromPR > 0 {
		if c, err := dispatcher.FetchPRContext(ctx, exec, repo, *fromPR); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "warn: --from-pr %d: %v\n", *fromPR, err)
		} else {
			prCtx = c
		}
	}
	if *fromIssue > 0 {
		if c, err := dispatcher.FetchIssueContext(ctx, exec, repo, *fromIssue); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "warn: --from-issue %d: %v\n", *fromIssue, err)
		} else {
			issueCtx = c
		}
	}
	if *fromThread != "" {
		histPath := defaultHistoryPath()
		if c, err := dispatcher.FetchThreadContext(*fromThread, histPath); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "warn: --from-thread %s: %v\n", *fromThread, err)
		} else {
			threadCtx = c
		}
	}
	composed := dispatcher.ComposeContext(prCtx, issueCtx, threadCtx)
	return runShipWithContext(ctx, repo, intent, composed)
}

func runShip(ctx context.Context, repo, intent string) int {
	return runShipWithContext(ctx, repo, intent, "")
}

func runShipWithContext(ctx context.Context, repo, intent, contextBlock string) int {
	auditPath := filepath.Join(stateDir(), "audit.jsonl")
	a := &audit.Logger{Path: auditPath, DefaultWorkspace: activeWorkspace}
	b := budget.New()

	issueTpl, err := os.ReadFile(filepath.Join(promptDir(), "regatta-issue.md"))
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "read issue template: %v\n", err)
		return 1
	}

	client, err := reasoner.NewAnthropicClient()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	r := &reasoner.Reasoner{Client: client, Budget: b, SystemPrompt: string(issueTpl), PersonaPrefix: personaPrefixForActive()}

	tmp, err := os.MkdirTemp("", "leah-ship-*")
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "tmp dir: %v\n", err)
		return 1
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	ship := &dispatcher.Ship{
		Reasoner:  r,
		GH:        ghclient.New(),
		Audit:     a,
		Budget:    b,
		Out:       os.Stdout,
		Repo:      repo,
		TmpDir:    tmp,
		Context:   contextBlock,
		Watch:     true,
		Regatta:   regattaclient.New(),
		Heartbeat: watchdog.New(),
		Notify:    commsout.NewDesktop(),
		PollEvery: 30 * time.Second,
		MaxPolls:  120, // 60 min max watch
	}
	if err := ship.Run(ctx, intent); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah ship: %v\n", err)
		return 1
	}
	return 0
}

// defaultHistoryPath returns ~/.zsh_history if $SHELL is zsh, else
// ~/.bash_history. Empty when $HOME is unset (FetchThreadContext skips).
func defaultHistoryPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	shell := os.Getenv("SHELL")
	if filepath.Base(shell) == "zsh" {
		return filepath.Join(home, ".zsh_history")
	}
	return filepath.Join(home, ".bash_history")
}

func runReview(ctx context.Context, repo string, prNum int) int {
	auditPath := filepath.Join(stateDir(), "audit.jsonl")
	a := &audit.Logger{Path: auditPath, DefaultWorkspace: activeWorkspace}
	b := budget.New()

	sysPrompt, err := os.ReadFile(filepath.Join(promptDir(), "reviewer-independent-reviewer.md"))
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "read reviewer prompt: %v\n", err)
		return 1
	}

	sub, err := reviewer.NewAnthropicSubagent()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah review: %v\n", err)
		return 1
	}
	sink := newFlushingSink(os.Stdout)
	r := &reviewer.Reviewer{Subagent: sub, Budget: b, SystemPrompt: string(sysPrompt), TokenSink: sink}

	gh := ghclient.New()
	pr, err := gh.ViewPR(ctx, repo, prNum,
		[]string{"number", "title", "body", "headRefName", "url"})
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "view pr: %v\n", err)
		return 1
	}

	diffOut, err := ghclient.ShellExec{}.Run(ctx,
		[]string{"gh", "pr", "diff", fmt.Sprintf("%d", prNum), "--repo", repo})
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "fetch diff: %v\n", err)
		return 1
	}

	body, _ := pr["body"].(string)
	v, err := r.Review(ctx, diffOut, body)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "review: %v\n", err)
		_ = a.Append(audit.Entry{Kind: "review", BlastRadius: 3, Outcome: "failed", Detail: err.Error()})
		return 1
	}

	// Body already streamed to stdout via TokenSink. Add a single newline
	// separator only when the stream didn't already end with one — avoids
	// the double blank-line gap before the Verdict: footer.
	if !sink.EndedInNewline() {
		_, _ = fmt.Println()
	}
	_, _ = fmt.Println("Verdict:", v.Recommendation, " Agent-id:", v.AgentID)

	_ = a.Append(audit.Entry{
		Kind:        "review",
		ArgsHash:    fmt.Sprintf("pr-%d", prNum),
		BlastRadius: 3,
		Outcome:     "success",
		CostDollars: b.Spent(),
		Detail:      v.Recommendation + " " + v.AgentID,
	})
	return 0
}

// personaPrefixForActive loads the persona row for the operator's active
// workspace and returns its SystemPromptPrefix. Soft-fails to empty string
// on any error so a missing persona DB / corrupted row never blocks the
// user-facing CLI invocation. The Reasoner treats empty prefix as legacy
// behavior (SystemPrompt unchanged).
func personaPrefixForActive() string {
	ws := activeWorkspace()
	ps, err := persona.Open(memoryPath())
	if err != nil {
		return ""
	}
	defer func() { _ = ps.Close() }()
	p, err := ps.Load(ws)
	if err != nil {
		return ""
	}
	return p.SystemPromptPrefix()
}

// stateDirE resolves and ensures the per-operator state directory. Returns
// an error rather than calling os.Exit so the caller's defer chain
// (writeInterruptedAudit, snapshotCLIMetrics) is not bypassed.
func stateDirE() (string, error) {
	d := os.Getenv("LEAH_STATE_DIR")
	if d == "" {
		home, _ := os.UserHomeDir()
		d = filepath.Join(home, ".leah-state")
	}
	if err := os.MkdirAll(d, 0o700); err != nil {
		return "", fmt.Errorf("mkdir state dir: %w", err)
	}
	return d, nil
}

// stateDir is the legacy convenience wrapper. Panics on failure rather than
// os.Exit so the run()-level defer chain (writeInterruptedAudit,
// snapshotCLIMetrics) still fires before the process tears down. New code
// should call stateDirE and propagate the error.
func stateDir() string {
	d, err := stateDirE()
	if err != nil {
		// Log to stderr BEFORE panic so user sees the context instead of
		// just an unfiltered stack trace. Panic still propagates to unwind
		// defers (writeInterruptedAudit, snapshotCLIMetrics) per design.
		_, _ = fmt.Fprintf(os.Stderr, "leah: fatal: state dir init failed: %v\n", err)
		panic(err)
	}
	return d
}

func promptDir() string {
	if d := os.Getenv("LEAH_PROMPT_DIR"); d != "" {
		return d
	}
	return "prompts"
}

func usage() {
	_, _ = fmt.Fprintln(os.Stderr, "Leah — personal chief-of-staff")
	_, _ = fmt.Fprintln(os.Stderr, "")
	_, _ = fmt.Fprintln(os.Stderr, "usage: leah <command> [args...]")
	_, _ = fmt.Fprintln(os.Stderr, "")
	_, _ = fmt.Fprintln(os.Stderr, "commands:")
	_, _ = fmt.Fprintln(os.Stderr, "  init [--force]            first-launch wizard (plist + adapters)")
	_, _ = fmt.Fprintln(os.Stderr, "  ask \"<query>\"             ask a question and stream the answer")
	_, _ = fmt.Fprintln(os.Stderr, "  ship [--from-pr N] [--from-issue N] [--from-thread Wc|Wm] <repo> \"<intent>\"  file regatta issue + watch + narrate")
	_, _ = fmt.Fprintln(os.Stderr, "  call <callee> [--audio]   initiate a FaceTime video call (default) or audio call with [--audio]")
	_, _ = fmt.Fprintln(os.Stderr, "  review <repo> <pr#>       independent review verdict on a pull request")
	_, _ = fmt.Fprintln(os.Stderr, "  status [--json]           recent activity from audit log")
	_, _ = fmt.Fprintln(os.Stderr, "  contact <add|list|show>   manage contacts (memory)")
	_, _ = fmt.Fprintln(os.Stderr, "  project <add|list|show>   manage projects (memory)")
	_, _ = fmt.Fprintln(os.Stderr, "  decision <add|list|show>  log + recall decisions (memory)")
	_, _ = fmt.Fprintln(os.Stderr, "  ctx <new|switch|show|history|list>  context manager")
	_, _ = fmt.Fprintln(os.Stderr, "  workspace <list|new|switch|show|persona>  workspace + per-workspace persona")
	_, _ = fmt.Fprintln(os.Stderr, "  mistake add --audit-id <id> --root-cause <tag> --prevention <text>")
	_, _ = fmt.Fprintln(os.Stderr, "  retro [--week YYYY-WW]    weekly retro markdown")
	_, _ = fmt.Fprintln(os.Stderr, "  patterns [--weekly]       skill-candidate clusters from audit")
	_, _ = fmt.Fprintln(os.Stderr, "  suggest [--context X] [--llm]   suggest what to do next based on your recent activity")
	_, _ = fmt.Fprintln(os.Stderr, "  backlog [repo] [--json]   active agents + ready issues + recent PRs")
	_, _ = fmt.Fprintln(os.Stderr, "  recall [--llm] <query>    search your history and memory")
	_, _ = fmt.Fprintln(os.Stderr, "  self-build \"<intent>\"     dispatch a regatta self-build PR")
	_, _ = fmt.Fprintln(os.Stderr, "  cost [--since D] [--by kind|day|model] [--json]  aggregate spend")
	_, _ = fmt.Fprintln(os.Stderr, "  brief [--voice] [--silent]   daily morning brief (recap + backlog + recs + cost)")
	_, _ = fmt.Fprintln(os.Stderr, "  listen [--duration D] [--model M] [--repo R]   push-to-talk → whisper.cpp → intent dispatch")
	_, _ = fmt.Fprintln(os.Stderr, "  backup [--target local|b2|both] [--restore [--restore-to PATH]] [--verify]   restic snapshot of state dir")
	_, _ = fmt.Fprintln(os.Stderr, "  connect <integration>|--list  first-launch OAuth device-code for shipped adapters (gmail, gcal)")
	_, _ = fmt.Fprintln(os.Stderr, "  disconnect <integration>|--list  revoke + remove on-disk token for a shipped adapter")
	_, _ = fmt.Fprintln(os.Stderr, "  forget <pattern-id|all> [--dry-run] [--yes]  wipe learned patterns from your recommendations")
	_, _ = fmt.Fprintln(os.Stderr, "  purge --everything        BR=4: OAuth revoke + rm -rf ~/.leah-state/ + brew/PATH hints")
	_, _ = fmt.Fprintln(os.Stderr, "  news                      synthesized daily news digest (RSS sources)")
	_, _ = fmt.Fprintln(os.Stderr, "  oncall [--since D] [--kind SUBSTR]  alarming audit signals from last 1h (default)")
	_, _ = fmt.Fprintln(os.Stderr, "  paper <save|list|read>    arXiv read-later queue (id or arxiv.org URL)")
	_, _ = fmt.Fprintln(os.Stderr, "  quote <symbol>...         market quotes for given symbols (Alpha Vantage)")
	_, _ = fmt.Fprintln(os.Stderr, "  watch [<sym>|--rm <sym>]  manage watchlist symbols read by the morning brief")
	_, _ = fmt.Fprintln(os.Stderr, "  whoami [--full]           print workspace + integrations, or enumerate persisted state (M1)")
	_, _ = fmt.Fprintln(os.Stderr, "  export --all [--out PATH]  encrypted archive of ~/.leah-state (M3)")
	_, _ = fmt.Fprintln(os.Stderr, "  import <archive> [--overwrite]  restore encrypted archive (M3)")
	_, _ = fmt.Fprintln(os.Stderr, "  inbound enroll <channel> <peerID>  one-time loopback authorization for remote replies (F3)")
	_, _ = fmt.Fprintln(os.Stderr, "  self-upgrade              attested rebuild + atomic symlink-swap of ~/bin/leah (BR=4)")
	_, _ = fmt.Fprintln(os.Stderr, "  pr-state <N>|--open|--queue  one-line PR readiness (state, CI, review, mergeable)")
	_, _ = fmt.Fprintln(os.Stderr, "  review-queue [--org X] [--json]  PRs awaiting your review, oldest-first")
	_, _ = fmt.Fprintln(os.Stderr, "  open <target>             launch streaming/social via macOS open (netflix, spotify, linkedin, …)")
	_, _ = fmt.Fprintln(os.Stderr, "  find [--region XX] <title...>  which streaming services carry a title (TMDB)")
	_, _ = fmt.Fprintln(os.Stderr, "  strategist <post|next|inbox|queue|doctor>  social-post pipeline (text+image+clip via higgsfield)")
	_, _ = fmt.Fprintln(os.Stderr, "  keychain <set|get|delete> <service>  bridge Go reads to Swift wizard Keychain slot (stdin secret)")
	_, _ = fmt.Fprintln(os.Stderr, "  version                   show version")
}
