package eval

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type stubAsker struct {
	reply string
	err   error
}

func (s stubAsker) Ask(_ context.Context, _ string) (string, error) {
	return s.reply, s.err
}

func writeFixture(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

func TestScheduler_RunOnce_ScoresFixturesAndPersists(t *testing.T) {
	fixDir := t.TempDir()
	writeFixture(t, fixDir, "greet.yaml", `
id: greet-1
question: "say hello"
expected_contains: ["hello"]
`)
	writeFixture(t, fixDir, "math.yaml", `
id: math-1
question: "two plus two"
expected_contains: ["four"]
`)

	store, err := OpenStore(filepath.Join(t.TempDir(), "eval.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = store.Close() }()

	sched := &Scheduler{
		Asker:       stubAsker{reply: "hello there"},
		FixturesDir: fixDir,
		Store:       store,
		Now:         func() time.Time { return time.Date(2026, 6, 22, 18, 0, 0, 0, time.UTC) },
	}
	runID, summary, err := sched.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if runID <= 0 {
		t.Fatalf("runID should be > 0")
	}
	if summary.Total != 2 {
		t.Fatalf("Total: want 2, got %d", summary.Total)
	}
	if summary.Passed != 1 {
		t.Fatalf("Passed: want 1 (greet pass, math fail on stub), got %d", summary.Passed)
	}

	rows, err := store.ListResults(context.Background(), runID)
	if err != nil {
		t.Fatalf("list results: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("want 2 result rows, got %d", len(rows))
	}
}

func TestScheduler_LoadFixtures_RejectsUnknownFields(t *testing.T) {
	fixDir := t.TempDir()
	writeFixture(t, fixDir, "bad.yaml", `
id: bad-1
question: "q"
expected_contains: ["x"]
unknown_field: "smuggled"
`)
	if _, err := loadFixtures(fixDir); err == nil {
		t.Fatalf("expected error for unknown field, got nil")
	}
}

func TestScheduler_LoadFixtures_SkipsNonYaml(t *testing.T) {
	fixDir := t.TempDir()
	writeFixture(t, fixDir, "README.md", "not a fixture\n")
	writeFixture(t, fixDir, "ok.yaml", `
id: ok-1
question: "q"
expected_contains: ["x"]
`)
	got, err := loadFixtures(fixDir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 1 || got[0].ID != "ok-1" {
		t.Fatalf("want 1 fixture ok-1, got %+v", got)
	}
}

func TestScheduler_RunLoop_RespectsContextCancel(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "eval.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = store.Close() }()

	sched := &Scheduler{
		Asker:       stubAsker{reply: "x"},
		FixturesDir: t.TempDir(), // empty — RunOnce is a noop
		Store:       store,
		Interval:    10 * time.Millisecond,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	// Should return on ctx.Done; no error path should panic.
	sched.Run(ctx)
}
