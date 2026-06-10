package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// stateDir resolves ~/.leah-state (or LEAH_STATE_DIR override), creating it
// with 0o700 perms. Composition root for all per-daemon persistence paths
// (audit log, memory.db, panic dir, metrics, briefs, retros).
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
