package main

import (
	"fmt"
	"path/filepath"

	"github.com/trilam/leah/internal/ctxmgr"
	"github.com/trilam/leah/internal/memory"
)

// memoryPath is the canonical sqlite file used by memory + selflearn + ctxmgr.
// All three packages share one *sql.DB via memory.Store.DB(); schema_version 3
// owns contact/project/decision + context/operator_state/context_switch_log +
// mistake_log.
func memoryPath() string { return filepath.Join(stateDir(), "memory.db") }

// openMemoryStore opens (creating if needed) the shared memory DB at
// $LEAH_STATE_DIR/memory.db. Returns an error rather than os.Exit so the
// subcommand entrypoint can propagate exit codes through run() — letting
// the deferred audit-flush fire on SIGINT (review #55). Callers MUST defer Close.
func openMemoryStore() (*memory.Store, error) {
	s, err := memory.NewStore(memoryPath())
	if err != nil {
		return nil, fmt.Errorf("open memory store: %w", err)
	}
	return s, nil
}

// openCtxManager opens the ctxmgr handle against the same shared memory DB.
// Callers MUST defer Close.
func openCtxManager() (*ctxmgr.Manager, error) {
	m, err := ctxmgr.Open(memoryPath())
	if err != nil {
		return nil, fmt.Errorf("open ctxmgr: %w", err)
	}
	return m, nil
}

// hasFlag returns true if any arg in args equals flag. Used for parsing
// trailing --json on simple list/show subcommands that don't bother with a
// dedicated FlagSet.
func hasFlag(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}

// shouldShowHelp returns true when args contain `--help` or `-h` anywhere.
// Subcommand handlers MUST call this before any LLM dispatch or
// state-changing op so `leah <cmd> --help` prints usage instead of being
// forwarded to the Reasoner as intent text (Defect-3: self-build --help
// burned $0.0059 returning clarifying questions). Exact-match only —
// substrings like `--helpme` or bare words must not trip the gate.
func shouldShowHelp(args []string) bool {
	for _, a := range args {
		if a == "--help" || a == "-h" {
			return true
		}
	}
	return false
}
