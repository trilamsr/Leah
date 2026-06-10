package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/connect"
)

// whoamiSources is the closed set of persistent stores M1 enumerates.
// Adding a new persistent store MUST extend this slice — TestWhoamiFull_
// ClosedSetEnum_NoUnknownSource gates drift.
var whoamiSources = []string{
	"memory",
	"recommend",
	"knowledge",
	"macos_mirror",
	"events",
	"audit",
}

// whoamiRow is one JSON-lines record emitted by --full. Stable schema —
// downstream tooling pipes to jq and depends on the field set.
type whoamiRow struct {
	Source       string `json:"source"`
	Count        int64  `json:"count"`
	Path         string `json:"path"`
	LastModified string `json:"last_modified,omitempty"`
}

func runWhoami(ctx context.Context, args []string, w io.Writer) int {
	if shouldShowHelp(args) {
		printWhoamiUsage(w)
		return 0
	}
	full := false
	for _, a := range args {
		if a == "--full" {
			full = true
		}
	}
	if !full {
		return runWhoamiShort(w)
	}

	sd := stateDir()
	for _, src := range whoamiSources {
		row := probeSource(ctx, src, sd)
		b, err := json.Marshal(row)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "leah whoami: marshal %s: %v\n", src, err)
			return 1
		}
		_, _ = fmt.Fprintln(w, string(b))
	}

	// BR=2 audit row (spec §2.3) — row contents stay out; only the source list + total.
	a := &audit.Logger{Path: filepath.Join(sd, "audit.jsonl"), DefaultWorkspace: activeWorkspace}
	_ = a.Append(audit.Entry{
		Kind:        "whoami_full",
		BlastRadius: 2,
		Outcome:     "success",
		Detail:      strings.Join(whoamiSources, ","),
	})
	return 0
}

func runWhoamiShort(w io.Writer) int {
	_, _ = fmt.Fprintf(w, "workspace: %s\n", activeWorkspace())
	authorized := []string{}
	for _, s := range connect.DefaultRegistry().List() {
		if s.Authorized {
			authorized = append(authorized, s.Name)
		}
	}
	_, _ = fmt.Fprintf(w, "integrations: %s\n", strings.Join(authorized, ","))
	return 0
}

// probeSource returns the whoamiRow for src. Each source is graceful-empty:
// missing file → count=0, empty last_modified. No source ever errors the run.
func probeSource(_ context.Context, src, stateDirPath string) whoamiRow {
	switch src {
	case "memory":
		p := filepath.Join(stateDirPath, "memory.db")
		return sqliteRow(src, p, []string{"contact", "project", "decision", "operator_profile"})
	case "recommend":
		p := filepath.Join(stateDirPath, "recommend.db")
		return sqliteRow(src, p, []string{"recommendations"})
	case "knowledge":
		p := filepath.Join(stateDirPath, "knowledge.db")
		return sqliteRow(src, p, []string{"entities"})
	case "events":
		p := filepath.Join(stateDirPath, "events.db")
		return sqliteRow(src, p, []string{"events"})
	case "macos_mirror":
		return dirRow(src, filepath.Join(stateDirPath, "mirror"))
	case "audit":
		return jsonlRow(src, filepath.Join(stateDirPath, "audit.jsonl"))
	}
	return whoamiRow{Source: src, Path: stateDirPath}
}

// sqliteRow sums COUNT(*) across tables that exist in the DB (closed-set
// candidates per source). Missing file → rows=0. Missing table → skipped.
// This is read-only — DB stays untouched whether present or absent.
func sqliteRow(src, path string, tables []string) whoamiRow {
	r := whoamiRow{Source: src, Path: path}
	st, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return r
	}
	if err == nil {
		r.LastModified = st.ModTime().UTC().Format(time.RFC3339)
	}
	dsn := fmt.Sprintf("file:%s?mode=ro&_pragma=busy_timeout(2000)", url.PathEscape(path))
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return r
	}
	defer func() { _ = db.Close() }()
	for _, tbl := range tables {
		var n int64
		err := db.QueryRow(`SELECT COUNT(*) FROM ` + tbl).Scan(&n)
		if err != nil {
			continue
		}
		r.Count += n
	}
	return r
}

// dirRow counts entries under dir (one row per mirrored adapter file). Missing
// dir → rows=0. Sub-counts aren't disaggregated here — that's spec §2.1's
// `stream` dimension, deferred until a per-adapter mirror DB exists.
func dirRow(src, dir string) whoamiRow {
	r := whoamiRow{Source: src, Path: dir}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return r
	}
	r.Count = int64(len(entries))
	if st, serr := os.Stat(dir); serr == nil {
		r.LastModified = st.ModTime().UTC().Format(time.RFC3339)
	}
	return r
}

// jsonlRow counts newline-terminated rows in audit.jsonl. Operator-attested
// audit rows survive across leah versions, so a single-pass byte scan beats
// loading the file (multi-MB after a week of dispatch).
func jsonlRow(src, path string) whoamiRow {
	r := whoamiRow{Source: src, Path: path}
	st, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return r
	}
	if err == nil {
		r.LastModified = st.ModTime().UTC().Format(time.RFC3339)
	}
	f, err := os.Open(path)
	if err != nil {
		return r
	}
	defer func() { _ = f.Close() }()
	buf := make([]byte, 32*1024)
	for {
		n, err := f.Read(buf)
		for i := 0; i < n; i++ {
			if buf[i] == '\n' {
				r.Count++
			}
		}
		if err != nil {
			break
		}
	}
	return r
}

func printWhoamiUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "usage: leah whoami [--full]")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Without flags: print active workspace + authorized integrations.")
	_, _ = fmt.Fprintln(w, "--full: emit JSON-lines, one row per persisted source.")
}
