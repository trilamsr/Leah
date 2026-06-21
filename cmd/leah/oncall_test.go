package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestRunOncall_PrintsAlarmingRows asserts the human surface lists kind +
// outcome + truncated detail for each alarming row in the window.
func TestRunOncall_PrintsAlarmingRows(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	now := time.Now().UTC()
	lines := []string{
		`{"ts":"` + now.Format(time.RFC3339) + `","kind":"ship","outcome":"failed","detail":"diff empty"}`,
		`{"ts":"` + now.Format(time.RFC3339) + `","kind":"ask","outcome":"success"}`,
		`{"ts":"` + now.Format(time.RFC3339) + `","kind":"breaker.tripped","outcome":"ok","detail":"cost cap"}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	code := runOncall(path, []string{}, &out)
	if code != 0 {
		t.Fatalf("exit %d, want 0; out=%q", code, out.String())
	}
	got := out.String()
	if !strings.Contains(got, "ship") || !strings.Contains(got, "failed") {
		t.Fatalf("missing ship/failed row: %q", got)
	}
	if !strings.Contains(got, "breaker.tripped") {
		t.Fatalf("missing breaker.tripped row: %q", got)
	}
	if strings.Contains(got, "\"kind\":\"ask\"") || strings.Contains(got, " ask ") {
		t.Fatalf("non-alarming ask row leaked: %q", got)
	}
}

// TestRunOncall_EmptyShowsAllClear keeps the operator's "nothing to do"
// path explicit — silence here would look like the command crashed.
func TestRunOncall_EmptyShowsAllClear(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	if err := os.WriteFile(path, []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	code := runOncall(path, []string{}, &out)
	if code != 0 {
		t.Fatalf("exit %d, want 0", code)
	}
	if !strings.Contains(out.String(), "all clear") {
		t.Fatalf("want 'all clear', got %q", out.String())
	}
}

// TestRunOncall_SinceFlag widens the window from the 1h default.
func TestRunOncall_SinceFlag(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	oldTS := time.Now().UTC().Add(-3 * time.Hour).Format(time.RFC3339)
	lines := []string{
		`{"ts":"` + oldTS + `","kind":"ship","outcome":"failed","detail":"old"}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var defaultOut bytes.Buffer
	if code := runOncall(path, nil, &defaultOut); code != 0 {
		t.Fatalf("default exit %d", code)
	}
	if strings.Contains(defaultOut.String(), "ship") {
		t.Fatalf("3h-old row leaked into default 1h window: %q", defaultOut.String())
	}
	var wideOut bytes.Buffer
	if code := runOncall(path, []string{"--since", "24h"}, &wideOut); code != 0 {
		t.Fatalf("24h exit %d", code)
	}
	if !strings.Contains(wideOut.String(), "ship") {
		t.Fatalf("24h window dropped row: %q", wideOut.String())
	}
}

// TestRunOncall_KindFilter narrows by Kind substring.
func TestRunOncall_KindFilter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	now := time.Now().UTC().Format(time.RFC3339)
	lines := []string{
		`{"ts":"` + now + `","kind":"breaker.tripped","outcome":"ok","detail":"cost"}`,
		`{"ts":"` + now + `","kind":"ship","outcome":"failed","detail":"x"}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	code := runOncall(path, []string{"--kind", "breaker"}, &out)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out.String(), "breaker") || strings.Contains(out.String(), " ship ") {
		t.Fatalf("kind filter wrong: %q", out.String())
	}
}

// TestRunOncall_BadSinceExits2 surfaces unparseable durations as a usage
// error so the operator doesn't think "0 results" means clean.
func TestRunOncall_BadSinceExits2(t *testing.T) {
	var out bytes.Buffer
	code := runOncall("/nonexistent", []string{"--since", "banana"}, &out)
	if code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
}
