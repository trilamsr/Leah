package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/trilam/leah/internal/ctxmgr"
	"github.com/trilam/leah/internal/memory"
)

// memoryPath is the canonical sqlite file used by memory + selflearn + ctxmgr.
// All three packages share one *sql.DB via memory.Store.DB() (see Wave 2-G
// schema reconciliation: schema_version 3 owns contact/project/decision +
// context/operator_state/context_switch_log + mistake_log).
func memoryPath() string { return filepath.Join(stateDir(), "memory.db") }

// openMemoryStore opens (creating if needed) the shared memory DB at
// $LEAH_STATE_DIR/memory.db. Callers MUST defer Close.
func openMemoryStore() *memory.Store {
	s, err := memory.NewStore(memoryPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "open memory store: %v\n", err)
		os.Exit(1)
	}
	return s
}

// openCtxManager opens the ctxmgr handle against the same shared memory DB.
// Callers MUST defer Close.
func openCtxManager() *ctxmgr.Manager {
	m, err := ctxmgr.Open(memoryPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "open ctxmgr: %v\n", err)
		os.Exit(1)
	}
	return m
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
