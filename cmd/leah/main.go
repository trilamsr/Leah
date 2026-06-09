package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/budget"
	"github.com/trilam/leah/internal/dispatcher"
	"github.com/trilam/leah/internal/ghclient"
	"github.com/trilam/leah/internal/notify"
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
		if len(os.Args) < 3 {
			_, _ = fmt.Fprintln(os.Stderr, "usage: leah ask \"<query>\"")
			os.Exit(2)
		}
		runAsk(os.Args[2])
	case "ship":
		if len(os.Args) < 4 {
			_, _ = fmt.Fprintln(os.Stderr, "usage: leah ship <repo> \"<intent>\"")
			os.Exit(2)
		}
		runShip(os.Args[2], os.Args[3])
	case "review":
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
		if len(os.Args) < 3 {
			_, _ = fmt.Fprintln(os.Stderr, "usage: leah self-build \"<intent>\"")
			os.Exit(2)
		}
		runSelfBuild(os.Args[2])
	default:
		usage()
		os.Exit(2)
	}
}

func runAsk(query string) {
	ctx := context.Background()

	auditPath := filepath.Join(stateDir(), "audit.jsonl")
	a := &audit.Logger{Path: auditPath}
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
	r := &reasoner.Reasoner{Client: client, Budget: b, SystemPrompt: string(systemPrompt)}

	ask := &dispatcher.Ask{Reasoner: r, Audit: a, Budget: b, Out: os.Stdout}
	if err := ask.Run(ctx, query); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah ask: %v\n", err)
		os.Exit(1)
	}
}

func runShip(repo, intent string) {
	ctx := context.Background()

	auditPath := filepath.Join(stateDir(), "audit.jsonl")
	a := &audit.Logger{Path: auditPath}
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
	r := &reasoner.Reasoner{Client: client, Budget: b, SystemPrompt: string(issueTpl)}

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

func runReview(repo string, prNum int) {
	ctx := context.Background()

	auditPath := filepath.Join(stateDir(), "audit.jsonl")
	a := &audit.Logger{Path: auditPath}
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
	_, _ = fmt.Fprintln(os.Stderr, "  ship <repo> \"<intent>\"    file regatta issue + watch + narrate")
	_, _ = fmt.Fprintln(os.Stderr, "  review <repo> <pr#>       independent reviewer subagent on PR")
	_, _ = fmt.Fprintln(os.Stderr, "  status [--json]           recent activity from audit log")
	_, _ = fmt.Fprintln(os.Stderr, "  contact <add|list|show>   manage contacts (memory)")
	_, _ = fmt.Fprintln(os.Stderr, "  project <add|list|show>   manage projects (memory)")
	_, _ = fmt.Fprintln(os.Stderr, "  decision <add|list|show>  log + recall decisions (memory)")
	_, _ = fmt.Fprintln(os.Stderr, "  ctx <new|switch|show|history|list>  context manager")
	_, _ = fmt.Fprintln(os.Stderr, "  mistake add --audit-id <id> --root-cause <tag> --prevention <text>")
	_, _ = fmt.Fprintln(os.Stderr, "  retro [--week YYYY-WW]    weekly retro markdown")
	_, _ = fmt.Fprintln(os.Stderr, "  patterns [--weekly]       skill-candidate clusters from audit")
	_, _ = fmt.Fprintln(os.Stderr, "  suggest [--context X] [--llm]   surface operator-model recommendations")
	_, _ = fmt.Fprintln(os.Stderr, "  backlog [repo] [--json]   active agents + ready issues + recent PRs")
	_, _ = fmt.Fprintln(os.Stderr, "  recall [--llm] <query>    semantic search over audit + memory")
	_, _ = fmt.Fprintln(os.Stderr, "  self-build \"<intent>\"     dispatch a regatta self-build PR")
	_, _ = fmt.Fprintln(os.Stderr, "  version                   show version")
}
