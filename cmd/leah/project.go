package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/memory"
)

// runProject dispatches `leah project <action> ...`.
func runProject(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: leah project <add|list|show> [args...]")
		os.Exit(2)
	}
	store := openMemoryStore()
	defer func() { _ = store.Close() }()
	a := &audit.Logger{Path: filepath.Join(stateDir(), "audit.jsonl")}

	switch args[0] {
	case "add":
		fs := flag.NewFlagSet("project add", flag.ExitOnError)
		name := fs.String("name", "", "project name (required)")
		status := fs.String("status", "active", "active|paused|done")
		notes := fs.String("notes", "", "free-form notes")
		jsonOut := fs.Bool("json", false, "emit json")
		_ = fs.Parse(args[1:])
		if *name == "" {
			fmt.Fprintln(os.Stderr, "leah project add: --name required")
			os.Exit(2)
		}
		p, err := store.AddProject(memory.Project{Name: *name, Status: *status, Notes: *notes})
		if err != nil {
			fmt.Fprintf(os.Stderr, "leah project add: %v\n", err)
			os.Exit(1)
		}
		_ = a.Append(audit.Entry{Kind: "project.add", ArgsHash: p.ID, BlastRadius: 1, Outcome: "success", Detail: p.Name})
		printProject(os.Stdout, p, *jsonOut)
	case "list":
		jsonOut := hasFlag(args[1:], "--json")
		ps, err := store.ListProjects()
		if err != nil {
			fmt.Fprintf(os.Stderr, "leah project list: %v\n", err)
			os.Exit(1)
		}
		_ = a.Append(audit.Entry{Kind: "project.list", BlastRadius: 0, Outcome: "success", Detail: fmt.Sprintf("count=%d", len(ps))})
		if jsonOut {
			_ = json.NewEncoder(os.Stdout).Encode(ps)
			return
		}
		if len(ps) == 0 {
			fmt.Println("(no projects)")
			return
		}
		for _, p := range ps {
			fmt.Printf("%s  [%s]  %s\n", p.ID, p.Status, p.Name)
		}
	case "show":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: leah project show <id> [--json]")
			os.Exit(2)
		}
		jsonOut := hasFlag(args[2:], "--json")
		p, err := store.GetProject(args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "leah project show: %v\n", err)
			os.Exit(1)
		}
		_ = a.Append(audit.Entry{Kind: "project.show", ArgsHash: p.ID, BlastRadius: 0, Outcome: "success"})
		printProject(os.Stdout, p, jsonOut)
	default:
		fmt.Fprintf(os.Stderr, "leah project: unknown action %q\n", args[0])
		os.Exit(2)
	}
}

func printProject(w io.Writer, p memory.Project, asJSON bool) {
	if asJSON {
		_ = json.NewEncoder(w).Encode(p)
		return
	}
	fmt.Fprintf(w, "id:        %s\n", p.ID)
	fmt.Fprintf(w, "name:      %s\n", p.Name)
	fmt.Fprintf(w, "status:    %s\n", p.Status)
	fmt.Fprintf(w, "notes:     %s\n", p.Notes)
	fmt.Fprintf(w, "created:   %s\n", p.CreatedAt)
	fmt.Fprintf(w, "updated:   %s\n", p.UpdatedAt)
}
