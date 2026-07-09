package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/backup"
	"github.com/trilam/leah/internal/daemonloop"
	commsout "github.com/trilam/leah/internal/comms/out"
)

// buildBackupSnapshotTask returns the weekly restic-snapshot task. Iterates
// configured + reachable repos (local mount + B2). Soft-fails when restic
// isn't installed or no repos are configured — operator runs
// `brew install restic && scripts/restic-init.sh` to enable. Per
// docs/research/2026-06-09-adopt-vs-build-survey.md §12 (restic 0.19.0,
// BSD-2).
func buildBackupSnapshotTask(sd string, a *audit.Logger, out *os.File) daemonloop.WeeklyTask {
	return func(ctx context.Context) {
		r := &backup.Restic{}
		if !r.Available() {
			return // silent — operator hasn't installed restic yet
		}
		repos := selectBackupRepos()
		if len(repos) == 0 {
			return // silent — operator hasn't configured a repo yet
		}
		for _, repo := range repos {
			if err := r.Snapshot(ctx, repo, sd); err != nil {
				_, _ = fmt.Fprintf(out, "leah-daemon: weekly backup %s error: %v\n", repo, err)
				_ = a.Append(audit.Entry{Kind: "backup.snapshot", ArgsHash: repo, BlastRadius: 2, Outcome: "failed", Detail: err.Error()})
				continue
			}
			_, _ = fmt.Fprintf(out, "leah-daemon: weekly backup %s: ok\n", repo)
			_ = a.Append(audit.Entry{Kind: "backup.snapshot", ArgsHash: repo, BlastRadius: 2, Outcome: "success"})
		}
	}
}

// buildBackupVerifyTask returns the quarterly verify-drill task
// (`restic check`). Only fires on the first Sunday of a calendar quarter
// (Jan / Apr / Jul / Oct). Inside the weekly tick because the daemon has
// no quarterly cadence primitive; the date gate makes it fire ~4×/year.
// Catches bit-rot before the operator needs the restore.
func buildBackupVerifyTask(a *audit.Logger, out *os.File) daemonloop.WeeklyTask {
	return func(ctx context.Context) {
		if !isFirstSundayOfQuarter(time.Now()) {
			return
		}
		r := &backup.Restic{}
		if !r.Available() {
			return
		}
		for _, repo := range selectBackupRepos() {
			if err := r.Verify(ctx, repo); err != nil {
				_, _ = fmt.Fprintf(out, "leah-daemon: quarterly verify %s error: %v\n", repo, err)
				_ = a.Append(audit.Entry{Kind: "backup.verify", ArgsHash: repo, BlastRadius: 1, Outcome: "failed", Detail: err.Error()})
				if d := commsout.NewDesktop(); d != nil {
					_ = d.Notify(ctx, "Leah: backup verify FAILED", fmt.Sprintf("repo %s — see audit log", repo))
				}
				continue
			}
			_, _ = fmt.Fprintf(out, "leah-daemon: quarterly verify %s: ok\n", repo)
			_ = a.Append(audit.Entry{Kind: "backup.verify", ArgsHash: repo, BlastRadius: 1, Outcome: "success"})
		}
	}
}

// selectBackupRepos returns local + B2 repos that are both configured AND
// (for local) currently mounted. Skipping unmounted USB volumes prevents
// restic from writing to a phantom mount point.
func selectBackupRepos() []string {
	var out []string
	local := os.Getenv("LEAH_BACKUP_LOCAL_PATH")
	if local == "" {
		local = "/Volumes/leah-backup"
	}
	if st, err := os.Stat(local); err == nil && st.IsDir() {
		out = append(out, local)
	}
	if b2 := os.Getenv("LEAH_BACKUP_B2_REPO"); b2 != "" {
		out = append(out, b2)
	}
	return out
}

// isFirstSundayOfQuarter reports whether t falls on the first Sunday of a
// quarter month (Jan, Apr, Jul, Oct). Gates the quarterly verify drill.
func isFirstSundayOfQuarter(t time.Time) bool {
	m := t.Month()
	if m != time.January && m != time.April && m != time.July && m != time.October {
		return false
	}
	return t.Weekday() == time.Sunday && t.Day() <= 7
}
