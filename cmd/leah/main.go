package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
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
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	cmd := os.Args[1]
	switch cmd {
	case "version", "-v", "--version":
		_, _ = fmt.Println(version)
		return
	case "ask":
		if len(os.Args) < 3 || shouldShowHelp(os.Args[2:]) {
			_, _ = fmt.Fprintln(os.Stderr, "usage: leah ask \"<query>\"")
			if len(os.Args) >= 3 && shouldShowHelp(os.Args[2:]) {
				return
			}
			os.Exit(2)
		}
		runAsk(os.Args[2])
	case "ship":
		runShipArgs(os.Args[2:])
	case "review":
		if shouldShowHelp(os.Args[2:]) {
			_, _ = fmt.Fprintln(os.Stderr, "usage: leah review <repo> <pr#>")
			return
		}
		if len(os.Args) < 4 {
			_, _ = fmt.Fprintln(os.Stderr, "usage: leah review <repo> <pr#>")
			os.Exit(2)
		}
		var prNum int
		if _, err := fmt.Sscanf(os.Args[3], "%d", &prNum); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, "pr# must be an integer")
			os.Exit(2)
		}
		runReview(os.Args[2], prNum)
	case "status":
		if shouldShowHelp(os.Args[2:]) {
			_, _ = fmt.Fprintln(os.Stderr, "usage: leah status [--json]")
			return
		}
		jsonMode := false
		for _, a := range os.Args[2:] {
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
			os.Exit(1)
		}
	case "contact":
		runContact(os.Args[2:])
	case "project":
		runProject(os.Args[2:])
	case "decision":
		runDecision(os.Args[2:])
	case "ctx":
		runCtx(os.Args[2:])
	case "workspace":
		runWorkspace(os.Args[2:])
	case "mistake":
		runMistake(os.Args[2:])
	case "retro":
		runRetro(os.Args[2:])
	case "patterns":
		runPatterns(os.Args[2:])
	case "suggest":
		runSuggest(os.Args[2:])
	case "backlog":
		runBacklog(os.Args[2:])
	case "recall":
		runRecall(os.Args[2:])
	case "self-build":
		if shouldShowHelp(os.Args[2:]) {
			_, _ = fmt.Fprintln(os.Stderr, "usage: leah self-build \"<intent>\"")
			return
		}
		if len(os.Args) < 3 {
			_, _ = fmt.Fprintln(os.Stderr, "usage: leah self-build \"<intent>\"")
			os.Exit(2)
		}
		runSelfBuild(os.Args[2])
	case "cost":
		runCost(os.Args[2:])
	case "brief":
		runBrief(os.Args[2:])
	case "listen":
		runListen(os.Args[2:])
	case "backup":
		runBackup(os.Args[2:])
	case "connect":
		os.Exit(runConnect(os.Args[2:], os.Stdout))
	default:
		usage()
		os.Exit(2)
	}
}

func runAsk(query string) {
	ctx := context.Background()

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
func runShipArgs(args []string) {
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
	ctx := context.Background()
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
	runShipWithContext(repo, intent, composed)
}

func runShip(repo, intent string) {
	runShipWithContext(repo, intent, "")
}

func runShipWithContext(repo, intent, contextBlock string) {
	ctx := context.Background()

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

func runReview(repo string, prNum int) {
	ctx := context.Background()

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
