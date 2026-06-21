package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/trilam/leah/internal/recommend/sources"
)

type fakeSeam struct {
	items []sources.WorkItem
	err   error
}

func (f *fakeSeam) StaleItems(_ context.Context, _ time.Duration) ([]sources.WorkItem, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.items, nil
}

func TestThreadsRanksByMostRecentAcrossTools(t *testing.T) {
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	seams := map[string]sources.WorkItemSeam{
		"jira": &fakeSeam{items: []sources.WorkItem{
			{Key: "ENG-1", Title: "stale jira", Updated: now.Add(-72 * time.Hour)},
		}},
		"linear": &fakeSeam{items: []sources.WorkItem{
			{Key: "LEA-9", Title: "recent linear", Updated: now.Add(-2 * time.Hour)},
		}},
		"confluence": &fakeSeam{items: []sources.WorkItem{
			{Key: "WIKI-42", Title: "mid confluence", Updated: now.Add(-12 * time.Hour)},
		}},
	}
	var buf bytes.Buffer
	code := runThreadsWith(context.Background(), threadsOpts{now: now, seams: seams}, nil, &buf)
	if code != 0 {
		t.Fatalf("exit=%d, want 0; out=%s", code, buf.String())
	}
	out := buf.String()
	iLinear := strings.Index(out, "LEA-9")
	iWiki := strings.Index(out, "WIKI-42")
	iJira := strings.Index(out, "ENG-1")
	if iLinear < 0 || iWiki < 0 || iJira < 0 {
		t.Fatalf("missing rows: %q", out)
	}
	if iLinear >= iWiki || iWiki >= iJira {
		t.Fatalf("rank wrong (want linear < confluence < jira): %q", out)
	}
}

func TestThreadsFilterByTool(t *testing.T) {
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	seams := map[string]sources.WorkItemSeam{
		"jira":   &fakeSeam{items: []sources.WorkItem{{Key: "ENG-1", Title: "j", Updated: now}}},
		"linear": &fakeSeam{items: []sources.WorkItem{{Key: "LEA-9", Title: "l", Updated: now}}},
	}
	var buf bytes.Buffer
	code := runThreadsWith(context.Background(), threadsOpts{now: now, seams: seams}, []string{"--tool", "jira"}, &buf)
	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	out := buf.String()
	if !strings.Contains(out, "ENG-1") || strings.Contains(out, "LEA-9") {
		t.Fatalf("--tool filter leaked: %q", out)
	}
}

func TestThreadsSinceDropsOlder(t *testing.T) {
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	seams := map[string]sources.WorkItemSeam{
		"jira": &fakeSeam{items: []sources.WorkItem{
			{Key: "OLD-1", Title: "old", Updated: now.Add(-48 * time.Hour)},
			{Key: "NEW-1", Title: "fresh", Updated: now.Add(-1 * time.Hour)},
		}},
	}
	var buf bytes.Buffer
	code := runThreadsWith(context.Background(), threadsOpts{now: now, seams: seams}, []string{"--since", "24h"}, &buf)
	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	out := buf.String()
	if !strings.Contains(out, "NEW-1") || strings.Contains(out, "OLD-1") {
		t.Fatalf("--since filter wrong: %q", out)
	}
}

func TestThreadsJSON(t *testing.T) {
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	seams := map[string]sources.WorkItemSeam{
		"jira": &fakeSeam{items: []sources.WorkItem{{Key: "ENG-1", Title: "j", Updated: now.Add(-1 * time.Hour)}}},
	}
	var buf bytes.Buffer
	code := runThreadsWith(context.Background(), threadsOpts{now: now, seams: seams}, []string{"--json"}, &buf)
	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	var rows []threadRow
	if err := json.Unmarshal(buf.Bytes(), &rows); err != nil {
		t.Fatalf("bad json: %v: %s", err, buf.String())
	}
	if len(rows) != 1 || rows[0].Tool != "jira" || rows[0].Key != "ENG-1" {
		t.Fatalf("rows=%+v", rows)
	}
}

func TestThreadsEmpty(t *testing.T) {
	var buf bytes.Buffer
	code := runThreadsWith(context.Background(), threadsOpts{now: time.Now(), seams: map[string]sources.WorkItemSeam{}}, nil, &buf)
	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	if !strings.Contains(buf.String(), "(no items)") {
		t.Fatalf("want empty marker, got %q", buf.String())
	}
}

func TestThreadsToolErrorIsolated(t *testing.T) {
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	seams := map[string]sources.WorkItemSeam{
		"jira":   &fakeSeam{err: errors.New("boom")},
		"linear": &fakeSeam{items: []sources.WorkItem{{Key: "LEA-9", Title: "l", Updated: now}}},
	}
	var buf, ebuf bytes.Buffer
	code := runThreadsWith(context.Background(), threadsOpts{now: now, seams: seams, stderr: &ebuf}, nil, &buf)
	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	if !strings.Contains(buf.String(), "LEA-9") {
		t.Fatalf("want linear row despite jira err: %q", buf.String())
	}
	if !strings.Contains(ebuf.String(), "jira") {
		t.Fatalf("want jira err on stderr: %q", ebuf.String())
	}
}

func TestThreadsUnknownTool(t *testing.T) {
	var buf bytes.Buffer
	code := runThreadsWith(context.Background(), threadsOpts{now: time.Now(), seams: map[string]sources.WorkItemSeam{}}, []string{"--tool", "wat"}, &buf)
	if code != 2 {
		t.Fatalf("exit=%d, want 2", code)
	}
}

func TestThreadsHelp(t *testing.T) {
	var buf bytes.Buffer
	code := runThreads(context.Background(), []string{"--help"}, &buf)
	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	if !strings.Contains(buf.String(), "leah threads") {
		t.Fatalf("help missing usage: %q", buf.String())
	}
}
