package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/budget"
	"github.com/trilam/leah/internal/dispatcher"
	"github.com/trilam/leah/internal/ghclient"
	"github.com/trilam/leah/internal/notify"
	"github.com/trilam/leah/internal/persona"
	"github.com/trilam/leah/internal/reasoner"
	"github.com/trilam/leah/internal/regattaclient"
	"github.com/trilam/leah/internal/reviewer"
	"github.com/trilam/leah/internal/watchdog"
)

const version = "0.0.1-mvp5"

func main() {
	// One signal-aware ctx for the whole process — every subcommand shares it
	// so Ctrl-C cancels in-flight work AND triggers writeInterruptedAudit
	// before exit (BB-RETRO L1 / #16: previously each runX called
	// context.Background() and the audit row was dropped on SIGINT).
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	auditPath := filepath.Join(stateDir(), "audit.jsonl")
	defer writeInterruptedAudit(ctx, auditPath)

	os.Exit(runCommand(ctx, os.Args[1:]))
}

// runCommand dispatches argv (without the program name) under a shared ctx.
// Extracted from main() so tests can drive the dispatcher with an arbitrary
// (possibly canceled) ctx without spawning a subprocess.
func runCommand(ctx context.Context, args []string) int {
	if len(args) < 1 {
		usage()
		return 2
	}

	cmd := args[0]
	rest := args[1:]
	switch cmd {
	case "version", "-v", "--version":
		_, _ = fmt.Println(version)
		return 0
	case "ask":
		if len(rest) < 1 || shouldShowHelp(rest) {
			_, _ = fmt.Fprintln(os.Stderr, "usage: leah ask \"<query>\"")
			if len(rest) >= 1 && shouldShowHelp(rest) {
				return 0
			}
			return 2
		}
		runAsk(ctx, rest[0])
	case "ship":
		runShipArgs(ctx, rest)
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
		runReview(ctx, rest[0], prNum)
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
			Out:       os.Stdout,
			Limit:     20,
			JSON:      jsonMode,
		}
		if err := s.Run(); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "leah status: %v\n", err)
			return 1
		}
	case "contact":
		runContact(rest)
	case "project":
		runProject(rest)
	case "decision":
		runDecision(rest)
	case "ctx":
		runCtx(rest)
	case "workspace":
		runWorkspace(rest)
	case "mistake":
		runMistake(rest)
	case "retro":
		runRetro(rest)
	case "patterns":
		runPatterns(rest)
	case "suggest":
		runSuggest(ctx, rest)
	case "backlog":
		runBacklog(ctx, rest)
	case "recall":
		runRecall(ctx, rest)
	case "self-build":
		if shouldShowHelp(rest) {
			_, _ = fmt.Fprintln(os.Stderr, "usage: leah self-build \"<intent>\"")
			return 0
		}
		if len(rest) < 1 {
			_, _ = fmt.Fprintln(os.Stderr, "usage: leah self-build \"<intent>\"")
			return 2
		}
		runSelfBuild(ctx, rest[0])
	case "cost":
		runCost(rest)
	case "brief":
		runBrief(ctx, rest)
	case "listen":
		runListen(ctx, rest)
	case "backup":
		runBackup(ctx, rest)
	case "connect":
		return runConnect(ctx, rest, os.Stdout)
	default:
		usage()
		return 2
	}
	return 0
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

func runAsk(ctx context.Context, query string) {
	auditPath := filepath.Join(stateDir(), "audit.jsonl")
	a := &audit.Logger{Path: auditPath, DefaultWorkspace: activeWorkspace}
	b := budget.New()

	systemPrompt, err := os.ReadFile(filepath.Join(promptDir(), "system.md"))
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "read system prompt: %v\n", err)
		os.Exit(1)
	}

	client, err := reasoner.NewAnthropicClient()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	r := &reasoner.Reasoner{Client: client, Budget: b, SystemPrompt: string(systemPrompt), PersonaPrefix: personaPrefixForActive()}

	ask := &dispatcher.Ask{Reasoner: r, Audit: a, Budget: b, Out: os.Stdout}
	if err := ask.Run(ctx, query); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah ask: %v\n", err)
		os.Exit(1)
	}
}

// runShipArgs parses the `leah ship` flag set + dispatches. Supports:
//
//	leah ship [--from-pr N] [--from-issue N] [--from-thread Wc|Wm] <repo> "<intent>"
//
// Each --from-* flag fetches a context block (via gh / shell history); blocks
// are composed in PR → issue → thread order and prepended to the Reasoner
// draft prompt so the model sees the referenced artifacts before drafting.
func runShipArgs(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("ship", flag.ExitOnError)
	fromPR := fs.Int("from-pr", 0, "prepend gh pr view + diff for PR #N from the same repo")
	fromIssue := fs.Int("from-issue", 0, "prepend gh issue view + comments for issue #N from the same repo")
	fromThread := fs.String("from-thread", "", "prepend last-N shell-history entries (e.g. 100c or 30m)")
	fs.Usage = func() {
		_, _ = fmt.Fprintln(os.Stderr, "usage: leah ship [--from-pr N] [--from-issue N] [--from-thread Wc|Wm] <repo> \"<intent>\"")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if fs.NArg() < 2 {
		fs.Usage()
		os.Exit(2)
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
	runShipWithContext(ctx, repo, intent, composed)
}

func runShip(ctx context.Context, repo, intent string) {
	runShipWithContext(ctx, repo, intent, "")
}

func runShipWithContext(ctx context.Context, repo, intent, contextBlock string) {
	auditPath := filepath.Join(stateDir(), "audit.jsonl")
	a := &audit.Logger{Path: auditPath, DefaultWorkspace: activeWorkspace}
	b := budget.New()

	issueTpl, err := os.ReadFile(filepath.Join(promptDir(), "regatta-issue.md"))
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "read issue template: %v\n", err)
		os.Exit(1)
	}

	client, err := reasoner.NewAnthropicClient()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	r := &reasoner.Reasoner{Client: client, Budget: b, SystemPrompt: string(issueTpl), PersonaPrefix: personaPrefixForActive()}

	tmp, err := os.MkdirTemp("", "leah-ship-*")
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "tmp dir: %v\n", err)
		os.Exit(1)
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
		Notify:    notify.NewDesktop(),
		PollEvery: 30 * time.Second,
		MaxPolls:  120, // 60 min max watch
	}
	if err := ship.Run(ctx, intent); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah ship: %v\n", err)
		os.Exit(1)
	}
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

func runReview(ctx context.Context, repo string, prNum int) {
	auditPath := filepath.Join(stateDir(), "audit.jsonl")
	a := &audit.Logger{Path: auditPath, DefaultWorkspace: activeWorkspace}
	b := budget.New()

	sysPrompt, err := os.ReadFile(filepath.Join(reviewerPromptDir(), "independent-reviewer.md"))
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "read reviewer prompt: %v\n", err)
		os.Exit(1)
	}

	sub, err := reviewer.NewAnthropicSubagent()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	r := &reviewer.Reviewer{Subagent: sub, Budget: b, SystemPrompt: string(sysPrompt)}

	gh := ghclient.New()
	pr, err := gh.ViewPR(ctx, repo, prNum,
		[]string{"number", "title", "body", "headRefName", "url"})
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "view pr: %v\n", err)
		os.Exit(1)
	}

	diffOut, err := ghclient.ShellExec{}.Run(ctx,
		[]string{"gh", "pr", "diff", fmt.Sprintf("%d", prNum), "--repo", repo})
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "fetch diff: %v\n", err)
		os.Exit(1)
	}

	body, _ := pr["body"].(string)
	v, err := r.Review(ctx, diffOut, body)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "review: %v\n", err)
		_ = a.Append(audit.Entry{Kind: "review", BlastRadius: 3, Outcome: "failed", Detail: err.Error()})
		os.Exit(1)
	}

	_, _ = fmt.Println(v.Body)
	_, _ = fmt.Println()
	_, _ = fmt.Println("Verdict:", v.Recommendation, " Agent-id:", v.AgentID)

	_ = a.Append(audit.Entry{
		Kind:        "review",
		ArgsHash:    fmt.Sprintf("pr-%d", prNum),
		BlastRadius: 3,
		Outcome:     "success",
		CostDollars: b.Spent(),
		Detail:      v.Recommendation + " " + v.AgentID,
	})
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

func stateDir() string {
	d := os.Getenv("LEAH_STATE_DIR")
	if d == "" {
		home, _ := os.UserHomeDir()
		d = filepath.Join(home, ".leah-state")
	}
	if err := os.MkdirAll(d, 0o700); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "mkdir state dir: %v\n", err)
		os.Exit(1)
	}
	return d
}

func promptDir() string {
	if d := os.Getenv("LEAH_PROMPT_DIR"); d != "" {
		return d
	}
	return "prompts"
}

func reviewerPromptDir() string {
	if d := os.Getenv("LEAH_REVIEWER_PROMPT_DIR"); d != "" {
		return d
	}
	return "reviewer-prompts"
}

func usage() {
	_, _ = fmt.Fprintln(os.Stderr, "Leah — personal AI chief-of-staff (MVP-5)")
	_, _ = fmt.Fprintln(os.Stderr, "")
	_, _ = fmt.Fprintln(os.Stderr, "usage: leah <command> [args...]")
	_, _ = fmt.Fprintln(os.Stderr, "")
	_, _ = fmt.Fprintln(os.Stderr, "commands:")
	_, _ = fmt.Fprintln(os.Stderr, "  ask \"<query>\"             direct query to Reasoner")
	_, _ = fmt.Fprintln(os.Stderr, "  ship [--from-pr N] [--from-issue N] [--from-thread Wc|Wm] <repo> \"<intent>\"  file regatta issue + watch + narrate")
	_, _ = fmt.Fprintln(os.Stderr, "  review <repo> <pr#>       independent reviewer subagent on PR")
	_, _ = fmt.Fprintln(os.Stderr, "  status [--json]           recent activity from audit log")
	_, _ = fmt.Fprintln(os.Stderr, "  contact <add|list|show>   manage contacts (memory)")
	_, _ = fmt.Fprintln(os.Stderr, "  project <add|list|show>   manage projects (memory)")
	_, _ = fmt.Fprintln(os.Stderr, "  decision <add|list|show>  log + recall decisions (memory)")
	_, _ = fmt.Fprintln(os.Stderr, "  ctx <new|switch|show|history|list>  context manager")
	_, _ = fmt.Fprintln(os.Stderr, "  workspace <list|new|switch|show|persona>  workspace + per-workspace persona")
	_, _ = fmt.Fprintln(os.Stderr, "  mistake add --audit-id <id> --root-cause <tag> --prevention <text>")
	_, _ = fmt.Fprintln(os.Stderr, "  retro [--week YYYY-WW]    weekly retro markdown")
	_, _ = fmt.Fprintln(os.Stderr, "  patterns [--weekly]       skill-candidate clusters from audit")
	_, _ = fmt.Fprintln(os.Stderr, "  suggest [--context X] [--llm]   surface operator-model recommendations")
	_, _ = fmt.Fprintln(os.Stderr, "  backlog [repo] [--json]   active agents + ready issues + recent PRs")
	_, _ = fmt.Fprintln(os.Stderr, "  recall [--llm] <query>    semantic search over audit + memory")
	_, _ = fmt.Fprintln(os.Stderr, "  self-build \"<intent>\"     dispatch a regatta self-build PR")
	_, _ = fmt.Fprintln(os.Stderr, "  cost [--since D] [--by kind|day|model] [--json]  aggregate spend")
	_, _ = fmt.Fprintln(os.Stderr, "  brief [--voice] [--silent]   daily morning brief (recap + backlog + recs + cost)")
	_, _ = fmt.Fprintln(os.Stderr, "  listen [--duration D] [--model M] [--repo R]   push-to-talk → whisper.cpp → intent dispatch")
	_, _ = fmt.Fprintln(os.Stderr, "  backup [--target local|b2|both] [--restore [--restore-to PATH]] [--verify]   restic snapshot of state dir")
	_, _ = fmt.Fprintln(os.Stderr, "  backup [--target local|b2|both] [--restore [--restore-to PATH]] [--verify]  restic snapshot of ~/.leah-state")
	_, _ = fmt.Fprintln(os.Stderr, "  connect <integration>|--list  first-launch OAuth device-code for shipped adapters (gmail, gcal)")
	_, _ = fmt.Fprintln(os.Stderr, "  version                   show version")
}
