package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/backup"
)

// runBackup implements `leah backup [--target local|b2|both] [--restore [path]]`.
//
// Snapshot mode (default): writes a new restic snapshot to local and/or
// B2 repos. Repos are operator-pre-configured via `restic init` — see
// scripts/restic-init.sh.
//
// Restore mode (--restore): pulls `latest` from the FIRST available repo
// (local first, B2 fallback) into the named path (default ./leah-restore-<ts>).
// Restore intentionally writes to a scratch dir, never overlaying ~/.leah-state.
//
// Audit: every operation (snapshot / restore / verify) logs to audit.jsonl
// with BlastRadius=2 (state-mutating but reversible — the source is read-only).
func runBackup(parent context.Context, args []string) int {
	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
	target := fs.String("target", "both", "snapshot target: local|b2|both")
	restore := fs.Bool("restore", false, "restore latest snapshot instead of writing one")
	restorePath := fs.String("restore-to", "", "restore destination (default ./leah-restore-<ts>)")
	verify := fs.Bool("verify", false, "run `restic check` on target repos instead of snapshotting")
	fs.Usage = func() {
		_, _ = fmt.Fprintln(os.Stderr, "usage: leah backup")
		_, _ = fmt.Fprintln(os.Stderr, "  restore mode: --restore [--restore-to PATH]")
		_, _ = fmt.Fprintln(os.Stderr, "  snapshot mode (default): [--target local|b2|both] [--verify]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		fs.Usage()
		return 2
	}

	if *target != "local" && *target != "b2" && *target != "both" {
		_, _ = fmt.Fprintf(os.Stderr, "leah backup: unknown --target %q (expect local|b2|both)\n", *target)
		return 2
	}

	r := &backup.Restic{}
	if !r.Available() {
		_, _ = fmt.Fprintln(os.Stderr, "leah backup: restic not on PATH — run `brew install restic`")
		return 1
	}

	ctx, cancel := context.WithTimeout(parent, 30*time.Minute)
	defer cancel()

	sd := stateDir()
	a := &audit.Logger{Path: filepath.Join(sd, "audit.jsonl")}

	repos := selectRepos(*target)
	if len(repos) == 0 {
		_, _ = fmt.Fprintln(os.Stderr, "leah backup: no repos selected — set LEAH_BACKUP_LOCAL_PATH and/or LEAH_BACKUP_B2_REPO")
		return 2
	}

	switch {
	case *restore:
		dst := *restorePath
		if dst == "" {
			dst = "./leah-restore-" + time.Now().UTC().Format("20060102-150405")
		}
		if err := doRestore(ctx, r, repos, dst, a); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "leah backup --restore: %v\n", err)
			return 1
		}
		_, _ = fmt.Printf("restored latest snapshot → %s\n", dst)
	case *verify:
		failed := 0
		for _, repo := range repos {
			if err := r.Verify(ctx, repo); err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "verify %s: %v\n", repo, err)
				_ = a.Append(audit.Entry{Kind: "backup.verify", ArgsHash: repo, BlastRadius: 1, Outcome: "failed", Detail: err.Error()})
				failed++
				continue
			}
			_, _ = fmt.Printf("verify %s: ok\n", repo)
			_ = a.Append(audit.Entry{Kind: "backup.verify", ArgsHash: repo, BlastRadius: 1, Outcome: "success"})
		}
		if failed > 0 {
			return 1
		}
	default:
		failed := 0
		for _, repo := range repos {
			if err := r.Snapshot(ctx, repo, sd); err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "snapshot %s: %v\n", repo, err)
				_ = a.Append(audit.Entry{Kind: "backup.snapshot", ArgsHash: repo, BlastRadius: 2, Outcome: "failed", Detail: err.Error()})
				failed++
				continue
			}
			_, _ = fmt.Printf("snapshot %s: ok\n", repo)
			_ = a.Append(audit.Entry{Kind: "backup.snapshot", ArgsHash: repo, BlastRadius: 2, Outcome: "success"})
		}
		if failed > 0 {
			return 1
		}
	}
	return 0
}

// selectRepos returns the resolved repo URLs for the requested target,
// skipping any whose env-var is unset (operator only configured one tier).
func selectRepos(target string) []string {
	local := os.Getenv("LEAH_BACKUP_LOCAL_PATH")
	if local == "" {
		local = "/Volumes/leah-backup"
	}
	b2 := os.Getenv("LEAH_BACKUP_B2_REPO") // e.g. b2:leah-backup:state
	var out []string
	if (target == "local" || target == "both") && local != "" && dirExists(local) {
		out = append(out, local)
	}
	if (target == "b2" || target == "both") && b2 != "" {
		out = append(out, b2)
	}
	return out
}

// dirExists guards against snapshotting to a phantom mount path. Returning
// false silently skips the local tier when the USB / SMB volume isn't
// mounted — operator sees "no repos selected" rather than restic-init error.
func dirExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

// doRestore tries each repo in order, returning on first success. local
// repo is tried before B2 (LAN latency < internet) when both are listed.
func doRestore(ctx context.Context, r *backup.Restic, repos []string, dst string, a *audit.Logger) error {
	if err := os.MkdirAll(dst, 0o700); err != nil {
		return fmt.Errorf("mkdir %s: %w", dst, err)
	}
	var lastErr error
	for _, repo := range repos {
		err := r.Restore(ctx, repo, dst)
		if err == nil {
			_ = a.Append(audit.Entry{Kind: "backup.restore", ArgsHash: repo, BlastRadius: 2, Outcome: "success", Detail: dst})
			return nil
		}
		_ = a.Append(audit.Entry{Kind: "backup.restore", ArgsHash: repo, BlastRadius: 2, Outcome: "failed", Detail: err.Error()})
		lastErr = err
	}
	if lastErr == nil {
		return fmt.Errorf("no repos available")
	}
	return lastErr
}
