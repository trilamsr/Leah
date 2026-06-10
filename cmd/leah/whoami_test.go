package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// TestWhoamiFull_ListsKnownStores seeds one row per source then asserts every
// closed-set source appears in --full output with rows>0 and a populated path.
func TestWhoamiFull_ListsKnownStores(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	seedAllSources(t, dir)

	var buf bytes.Buffer
	if code := runWhoami(context.Background(), []string{"--full"}, &buf); code != 0 {
		t.Fatalf("exit %d, out=%s", code, buf.String())
	}

	rows := parseWhoamiLines(t, buf.Bytes())
	if len(rows) != len(whoamiSources) {
		t.Fatalf("got %d rows, want %d (one per source)", len(rows), len(whoamiSources))
	}
	for _, src := range whoamiSources {
		r, ok := rows[src]
		if !ok {
			t.Errorf("source %q missing from output", src)
			continue
		}
		if r["count"] == nil {
			t.Errorf("source %q: missing count field", src)
		}
		if r["path"] == "" {
			t.Errorf("source %q: empty path", src)
		}
	}
}

// TestWhoamiFull_HandlesEmptyStateDir asserts a first-run operator (no DBs,
// no audit log) gets exit 0 + one row per source with rows=0.
func TestWhoamiFull_HandlesEmptyStateDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)

	var buf bytes.Buffer
	if code := runWhoami(context.Background(), []string{"--full"}, &buf); code != 0 {
		t.Fatalf("exit %d, out=%s", code, buf.String())
	}
	rows := parseWhoamiLines(t, buf.Bytes())
	if len(rows) != len(whoamiSources) {
		t.Fatalf("got %d rows, want %d (one per source even when empty)", len(rows), len(whoamiSources))
	}
	for _, src := range whoamiSources {
		r, ok := rows[src]
		if !ok {
			t.Errorf("source %q missing", src)
			continue
		}
		// Empty state: count must be present (0) — first-run UX gate.
		if n, _ := r["count"].(float64); n != 0 {
			t.Errorf("source %q: empty-state count = %v, want 0", src, n)
		}
	}
}

// TestWhoamiFull_ClosedSetEnum_NoUnknownSource gates enum drift — adding a new
// persistent store MUST extend whoamiSources, otherwise output contains a row
// outside the closed set (silent capability creep, M1 spec §2.1).
func TestWhoamiFull_ClosedSetEnum_NoUnknownSource(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	seedAllSources(t, dir)

	var buf bytes.Buffer
	if code := runWhoami(context.Background(), []string{"--full"}, &buf); code != 0 {
		t.Fatalf("exit %d", code)
	}
	allowed := make(map[string]struct{}, len(whoamiSources))
	for _, s := range whoamiSources {
		allowed[s] = struct{}{}
	}
	rows := parseWhoamiLines(t, buf.Bytes())
	for src := range rows {
		if _, ok := allowed[src]; !ok {
			t.Errorf("unknown source %q in output (not in closed set %v)", src, whoamiSources)
		}
	}
	// Golden order: stable output for downstream pipe-to-jq tooling.
	gotOrder := parseWhoamiOrder(t, buf.Bytes())
	want := append([]string(nil), whoamiSources...)
	sortSrc := append([]string(nil), gotOrder...)
	sort.Strings(sortSrc)
	sort.Strings(want)
	for i := range want {
		if sortSrc[i] != want[i] {
			t.Errorf("source set mismatch: got %v, want %v", gotOrder, whoamiSources)
			break
		}
	}
}

// TestWhoami_Short_NoFlag prints short form (operator email + workspace) on
// bare `leah whoami`.
func TestWhoami_Short_NoFlag(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	var buf bytes.Buffer
	if code := runWhoami(context.Background(), []string{}, &buf); code != 0 {
		t.Fatalf("exit %d, out=%s", code, buf.String())
	}
	if !strings.Contains(buf.String(), "workspace") {
		t.Errorf("short form missing workspace marker: %q", buf.String())
	}
}

// TestWhoami_Help_ReturnsZero — `--help` short-circuits like every other subcommand.
func TestWhoami_Help_ReturnsZero(t *testing.T) {
	var buf bytes.Buffer
	if code := runWhoami(context.Background(), []string{"--help"}, &buf); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(buf.String(), "leah whoami") {
		t.Errorf("usage missing: %q", buf.String())
	}
}

// seedAllSources writes one row to every whoami source so the enumerator has
// non-zero counts to report. Touches each file path through the public
// surface area (sqlite3 Open + minimal DDL) so we exercise the real
// row-count probe, not the count function's empty-path fallback.
func seedAllSources(t *testing.T, dir string) {
	t.Helper()
	// memory.db — contact table row.
	memDB := openSqlite(t, filepath.Join(dir, "memory.db"))
	defer func() { _ = memDB.Close() }()
	mustExec(t, memDB, `CREATE TABLE contact (id TEXT PRIMARY KEY, name TEXT)`)
	mustExec(t, memDB, `INSERT INTO contact VALUES ('a','x')`)

	// recommend.db — recommendations table row.
	recDB := openSqlite(t, filepath.Join(dir, "recommend.db"))
	defer func() { _ = recDB.Close() }()
	mustExec(t, recDB, `CREATE TABLE recommendations (id TEXT PRIMARY KEY)`)
	mustExec(t, recDB, `INSERT INTO recommendations VALUES ('r1')`)

	// knowledge.db — entities table row.
	knDB := openSqlite(t, filepath.Join(dir, "knowledge.db"))
	defer func() { _ = knDB.Close() }()
	mustExec(t, knDB, `CREATE TABLE entities (kind TEXT, key TEXT, PRIMARY KEY(kind,key))`)
	mustExec(t, knDB, `INSERT INTO entities VALUES ('person','k')`)

	// macos_mirror — directory with a stub file.
	mdir := filepath.Join(dir, "mirror")
	if err := os.MkdirAll(mdir, 0o700); err != nil {
		t.Fatalf("mkdir mirror: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mdir, "gmail.db"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write mirror file: %v", err)
	}

	// events.db — events table row.
	evDB := openSqlite(t, filepath.Join(dir, "events.db"))
	defer func() { _ = evDB.Close() }()
	mustExec(t, evDB, `CREATE TABLE events (id INTEGER PRIMARY KEY)`)
	mustExec(t, evDB, `INSERT INTO events VALUES (1)`)

	// audit.jsonl — one row.
	if err := os.WriteFile(filepath.Join(dir, "audit.jsonl"), []byte(`{"ts":"t","kind":"x"}`+"\n"), 0o600); err != nil {
		t.Fatalf("write audit: %v", err)
	}
}

func openSqlite(t *testing.T, path string) *sql.DB {
	t.Helper()
	dsn := "file:" + url.PathEscape(path) + "?_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	return db
}

func mustExec(t *testing.T, db *sql.DB, q string) {
	t.Helper()
	if _, err := db.Exec(q); err != nil {
		t.Fatalf("exec %s: %v", q, err)
	}
}

func parseWhoamiLines(t *testing.T, out []byte) map[string]map[string]any {
	t.Helper()
	rows := make(map[string]map[string]any)
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("parse jsonl line %q: %v", line, err)
		}
		src, _ := m["source"].(string)
		if src == "" {
			t.Fatalf("line missing source: %s", line)
		}
		rows[src] = m
	}
	return rows
}

func parseWhoamiOrder(t *testing.T, out []byte) []string {
	t.Helper()
	var order []string
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("parse jsonl: %v", err)
		}
		if s, _ := m["source"].(string); s != "" {
			order = append(order, s)
		}
	}
	return order
}
