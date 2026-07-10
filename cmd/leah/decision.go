package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/trilam/leah/internal/platform/audit"
	"github.com/trilam/leah/internal/memory/store"
)

// runDecision dispatches `leah decision <action> ...`.
func runDecision(args []string) int {
	if shouldShowHelp(args) {
		_, _ = fmt.Fprintln(os.Stderr, "usage: leah decision <add|list|show> [args...]")
		return 0
	}
	if len(args) < 1 {
		_, _ = fmt.Fprintln(os.Stderr, "usage: leah decision <add|list|show> [args...]")
		return 2
	}
	store, err := openMemoryStore()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	defer func() { _ = store.Close() }()
	logger := &audit.Logger{Path: filepath.Join(stateDir(), "audit.jsonl")}

	switch args[0] {
	case "add":
		fs := flag.NewFlagSet("decision add", flag.ContinueOnError)
		topic := fs.String("topic", "", "decision topic (required)")
		choice := fs.String("choice", "", "choice picked (required)")
		text := fs.String("text", "", "rationale; if omitted, read from stdin")
		decidedAt := fs.String("decided-at", "", "RFC3339 timestamp; defaults to now")
		jsonOut := fs.Bool("json", false, "emit json")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if *topic == "" || *choice == "" {
			_, _ = fmt.Fprintln(os.Stderr, "leah decision add: --topic and --choice required")
			return 2
		}
		rationale := *text
		if rationale == "" {
			b, err := io.ReadAll(os.Stdin)
			if err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "read stdin: %v\n", err)
				return 1
			}
			rationale = strings.TrimSpace(string(b))
		}
		d, err := store.AddDecision(memory.Decision{
			Topic: *topic, Choice: *choice, Rationale: rationale, DecidedAt: *decidedAt,
		})
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "leah decision add: %v\n", err)
			return 1
		}
		_ = logger.Append(audit.Entry{Kind: "decision.add", ArgsHash: d.ID, BlastRadius: 1, Outcome: "success", Detail: d.Topic})
		printDecision(os.Stdout, d, *jsonOut)
	case "list":
		jsonOut := hasFlag(args[1:], "--json")
		ds, err := store.ListDecisions()
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "leah decision list: %v\n", err)
			return 1
		}
		_ = logger.Append(audit.Entry{Kind: "decision.list", BlastRadius: 0, Outcome: "success", Detail: fmt.Sprintf("count=%d", len(ds))})
		if jsonOut {
			_ = json.NewEncoder(os.Stdout).Encode(ds)
			return 0
		}
		if len(ds) == 0 {
			_, _ = fmt.Println("(no decisions)")
			return 0
		}
		for _, d := range ds {
			fmt.Printf("%s  [%s]  %s -> %s\n", d.ID, d.DecidedAt, d.Topic, d.Choice)
		}
	case "show":
		if len(args) < 2 {
			_, _ = fmt.Fprintln(os.Stderr, "usage: leah decision show <id> [--json]")
			return 2
		}
		jsonOut := hasFlag(args[2:], "--json")
		d, err := store.GetDecision(args[1])
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "leah decision show: %v\n", err)
			return 1
		}
		_ = logger.Append(audit.Entry{Kind: "decision.show", ArgsHash: d.ID, BlastRadius: 0, Outcome: "success"})
		printDecision(os.Stdout, d, jsonOut)
	default:
		_, _ = fmt.Fprintf(os.Stderr, "leah decision: unknown action %q\n", args[0])
		return 2
	}
	return 0
}

func printDecision(w io.Writer, d memory.Decision, asJSON bool) {
	if asJSON {
		_ = json.NewEncoder(w).Encode(d)
		return
	}
	_, _ = fmt.Fprintf(w, "id:         %s\n", d.ID)
	_, _ = fmt.Fprintf(w, "topic:      %s\n", d.Topic)
	_, _ = fmt.Fprintf(w, "choice:     %s\n", d.Choice)
	_, _ = fmt.Fprintf(w, "rationale:\n%s\n", d.Rationale)
	_, _ = fmt.Fprintf(w, "decided_at: %s\n", d.DecidedAt)
	_, _ = fmt.Fprintf(w, "created:    %s\n", d.CreatedAt)
}
