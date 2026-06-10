package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/daemonloop"
	"github.com/trilam/leah/internal/memory"
	"github.com/trilam/leah/internal/operatormodel"
)

// wireConsolidation hangs the nightly memory consolidation pass off the
// daily 3am tick. Co-exists with the morning-brief daily task (8am) — both
// land on loop.Daily and serialize through dailyMu. Kill-switch via
// LEAH_CONSOLIDATION=0 is enforced inside ConsolidatePass.Run so the
// daemon never has to special-case it here.
func wireConsolidation(loop *daemonloop.Loop, store *memory.Store, a *audit.Logger, auditPath, sd string) {
	if loop.DailyTracker == "" {
		loop.DailyTracker = filepath.Join(sd, "last-daily.txt")
	}
	// 3am is the deepest off-peak the brief daily tick can co-exist with.
	// The brief task gates separately on LEAH_BRIEF_HOUR (default 8) inside
	// its own closure, so the shared DailyHour stays 3am — earliest fire.
	loop.DailyHour = 3
	archivePath := filepath.Join(sd, "consolidated.jsonl")
	task := func(ctx context.Context) {
		cp := &operatormodel.ConsolidatePass{
			Store:       store,
			AuditPath:   auditPath,
			ArchivePath: archivePath,
			Audit:       a,
		}
		if err := cp.Run(ctx); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "leah-daemon: consolidation: %v\n", err)
		}
	}
	loop.Daily = append(loop.Daily, task)
}
