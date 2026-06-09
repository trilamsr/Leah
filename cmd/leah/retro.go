package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/selflearn"
)

// runRetro renders the weekly retro markdown to stdout.
func runRetro(args []string) {
	fs := flag.NewFlagSet("retro", flag.ExitOnError)
	week := fs.String("week", "", "ISO week YYYY-WW (defaults to current)")
	_ = fs.Parse(args)

	store, err := selflearn.OpenMistakeStore(memoryPath())
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah retro: open store: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = store.Close() }()

	auditPath := filepath.Join(stateDir(), "audit.jsonl")
	r := &selflearn.Retro{AuditPath: auditPath, Store: store}
	md, err := r.Generate(*week)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah retro: %v\n", err)
		os.Exit(1)
	}
	a := &audit.Logger{Path: auditPath}
	_ = a.Append(audit.Entry{Kind: "retro", ArgsHash: *week, BlastRadius: 0, Outcome: "success", Detail: *week})
	fmt.Print(md)
}
