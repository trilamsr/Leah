package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/selflearn"
)

// runMistake dispatches `leah mistake <action> ...`.
func runMistake(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: leah mistake add --audit-id <ts> --root-cause <tag> --prevention <text>")
		os.Exit(2)
	}
	switch args[0] {
	case "add":
		fs := flag.NewFlagSet("mistake add", flag.ExitOnError)
		// audit-id is the audit row's RFC3339 Timestamp; combined with the row's
		// (Kind, ArgsHash) it forms the composite key (spec §2). For the
		// personal-use CLI we keep this single flag and let the operator paste
		// any of the three values — the resolver will dedup on the composite.
		auditID := fs.String("audit-id", "", "RFC3339 timestamp of the audit row this mistake annotates")
		kind := fs.String("audit-kind", "", "kind of the audit row (e.g. \"ship\")")
		hash := fs.String("audit-hash", "", "args_hash of the audit row")
		rootCause := fs.String("root-cause", "", "short tag, e.g. wrong-pr (required)")
		prevention := fs.String("prevention", "", "free-form prevention note (required)")
		_ = fs.Parse(args[1:])
		if *auditID == "" || *rootCause == "" || *prevention == "" {
			fmt.Fprintln(os.Stderr, "leah mistake add: --audit-id, --root-cause, --prevention all required")
			os.Exit(2)
		}
		store, err := selflearn.OpenMistakeStore(memoryPath())
		if err != nil {
			fmt.Fprintf(os.Stderr, "leah mistake add: open store: %v\n", err)
			os.Exit(1)
		}
		defer func() { _ = store.Close() }()
		id, err := store.Add(selflearn.Mistake{
			AuditTS:    *auditID,
			AuditKind:  *kind,
			AuditHash:  *hash,
			RootCause:  *rootCause,
			Prevention: *prevention,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "leah mistake add: %v\n", err)
			os.Exit(1)
		}
		a := &audit.Logger{Path: filepath.Join(stateDir(), "audit.jsonl")}
		_ = a.Append(audit.Entry{Kind: "mistake.add", ArgsHash: id, BlastRadius: 1, Outcome: "success", Detail: *rootCause})
		fmt.Printf("logged mistake %s (root_cause=%q)\n", id, *rootCause)
	default:
		fmt.Fprintf(os.Stderr, "leah mistake: unknown action %q\n", args[0])
		os.Exit(2)
	}
}
