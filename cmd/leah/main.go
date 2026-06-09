package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/budget"
	"github.com/trilam/leah/internal/dispatcher"
	"github.com/trilam/leah/internal/reasoner"
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
		fmt.Println(version)
		return
	case "ask":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: leah ask \"<query>\"")
			os.Exit(2)
		}
		runAsk(os.Args[2])
	case "ship", "review", "status":
		fmt.Fprintf(os.Stderr, "subcommand %q not yet implemented\n", cmd)
		os.Exit(2)
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

	systemPrompt, err := os.ReadFile("prompts/system.md")
	if err != nil {
		fmt.Fprintf(os.Stderr, "read system prompt: %v\n", err)
		os.Exit(1)
	}

	client, err := reasoner.NewAnthropicClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	r := &reasoner.Reasoner{Client: client, Budget: b, SystemPrompt: string(systemPrompt)}

	ask := &dispatcher.Ask{Reasoner: r, Audit: a, Budget: b, Out: os.Stdout}
	if err := ask.Run(ctx, query); err != nil {
		fmt.Fprintf(os.Stderr, "leah ask: %v\n", err)
		os.Exit(1)
	}
}

func stateDir() string {
	d := os.Getenv("LEAH_STATE_DIR")
	if d == "" {
		home, _ := os.UserHomeDir()
		d = filepath.Join(home, ".leah-state")
	}
	if err := os.MkdirAll(d, 0o700); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir state dir: %v\n", err)
		os.Exit(1)
	}
	return d
}

func usage() {
	fmt.Fprintln(os.Stderr, "Leah — personal AI chief-of-staff (MVP-5)")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "usage: leah <command> [args...]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "commands:")
	fmt.Fprintln(os.Stderr, "  ask \"<query>\"        direct query to Reasoner")
	fmt.Fprintln(os.Stderr, "  ship \"<intent>\"      file regatta issue + watch + narrate")
	fmt.Fprintln(os.Stderr, "  review <pr#>         independent reviewer subagent on PR")
	fmt.Fprintln(os.Stderr, "  status               recent activity from audit log")
	fmt.Fprintln(os.Stderr, "  version              show version")
}
