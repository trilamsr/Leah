package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/trilam/leah/internal/audit"
)

// runCtx dispatches `leah ctx <action> ...`.
func runCtx(args []string) {
	if shouldShowHelp(args) {
		_, _ = fmt.Fprintln(os.Stderr, "usage: leah ctx <new|switch|show|history|list> [args...]")
		return
	}
	if len(args) < 1 {
		_, _ = fmt.Fprintln(os.Stderr, "usage: leah ctx <new|switch|show|history|list> [args...]")
		os.Exit(2)
	}
	mgr := openCtxManager()
	defer func() { _ = mgr.Close() }()
	// DefaultWorkspace pulls active workspace at append time so ctx.switch,
	// ctx.show etc. rows are tagged. After Switch executes, subsequent
	// Append calls observe the NEW active workspace — that's the desired
	// audit semantics ("the row was emitted while in workspace X").
	a := &audit.Logger{Path: filepath.Join(stateDir(), "audit.jsonl"), DefaultWorkspace: activeWorkspace}

	switch args[0] {
	case "new":
		fs := flag.NewFlagSet("ctx new", flag.ExitOnError)
		name := fs.String("name", "", "context name (lowercase, dash-allowed, 1-32 chars)")
		desc := fs.String("description", "", "human-readable description")
		_ = fs.Parse(args[1:])
		if *name == "" {
			_, _ = fmt.Fprintln(os.Stderr, "leah ctx new: --name required")
			os.Exit(2)
		}
		if err := mgr.NewContext(*name, *desc); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "leah ctx new: %v\n", err)
			os.Exit(1)
		}
		_ = a.Append(audit.Entry{Kind: "ctx.new", ArgsHash: *name, BlastRadius: 1, Outcome: "success", Detail: *name})
		fmt.Printf("created context %q\n", *name)
	case "switch":
		fs := flag.NewFlagSet("ctx switch", flag.ExitOnError)
		name := fs.String("name", "", "target context name")
		reason := fs.String("reason", "cli", "free-form reason recorded in switch log")
		_ = fs.Parse(args[1:])
		if *name == "" {
			_, _ = fmt.Fprintln(os.Stderr, "leah ctx switch: --name required")
			os.Exit(2)
		}
		if err := mgr.Switch(*name, *reason); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "leah ctx switch: %v\n", err)
			os.Exit(1)
		}
		_ = a.Append(audit.Entry{Kind: "ctx.switch", ArgsHash: *name, BlastRadius: 1, Outcome: "success", Detail: *name})
		fmt.Printf("switched to %q\n", *name)
	case "show":
		jsonOut := hasFlag(args[1:], "--json")
		c, err := mgr.Active()
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "leah ctx show: %v\n", err)
			os.Exit(1)
		}
		_ = a.Append(audit.Entry{Kind: "ctx.show", BlastRadius: 0, Outcome: "success", Detail: c.Name})
		if jsonOut {
			_ = json.NewEncoder(os.Stdout).Encode(c)
			return
		}
		fmt.Printf("active:      %s\n", c.Name)
		fmt.Printf("description: %s\n", c.Description)
		fmt.Printf("created:     %s\n", c.CreatedAt.Format("2006-01-02T15:04:05Z"))
	case "history":
		fs := flag.NewFlagSet("ctx history", flag.ExitOnError)
		limit := fs.Int("limit", 20, "max rows to show")
		jsonOut := fs.Bool("json", false, "emit json")
		_ = fs.Parse(args[1:])
		hist, err := mgr.History(*limit)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "leah ctx history: %v\n", err)
			os.Exit(1)
		}
		_ = a.Append(audit.Entry{Kind: "ctx.history", BlastRadius: 0, Outcome: "success", Detail: fmt.Sprintf("count=%d", len(hist))})
		if *jsonOut {
			_ = json.NewEncoder(os.Stdout).Encode(hist)
			return
		}
		if len(hist) == 0 {
			_, _ = fmt.Println("(no switches recorded)")
			return
		}
		for _, s := range hist {
			from := s.From
			if from == "" {
				from = "-"
			}
			fmt.Printf("%s  %s -> %s  (%s)\n",
				s.SwitchedAt.Format("2006-01-02T15:04:05Z"), from, s.To, s.Reason)
		}
	case "list":
		jsonOut := hasFlag(args[1:], "--json")
		cs, err := mgr.List()
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "leah ctx list: %v\n", err)
			os.Exit(1)
		}
		_ = a.Append(audit.Entry{Kind: "ctx.list", BlastRadius: 0, Outcome: "success", Detail: fmt.Sprintf("count=%d", len(cs))})
		if jsonOut {
			_ = json.NewEncoder(os.Stdout).Encode(cs)
			return
		}
		for _, c := range cs {
			fmt.Printf("%-20s  %s\n", c.Name, c.Description)
		}
	default:
		_, _ = fmt.Fprintf(os.Stderr, "leah ctx: unknown action %q\n", args[0])
		os.Exit(2)
	}
}
