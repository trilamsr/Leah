package recommend

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/trilam/leah/internal/audit"
)

// fakeOSExec records invocations and replays canned errors per call.
type fakeOSExec struct {
	calls []fakeCall
	errs  []error
}

type fakeCall struct {
	name string
	args []string
}

func (f *fakeOSExec) Run(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
	f.calls = append(f.calls, fakeCall{name: name, args: args})
	if len(f.errs) == 0 {
		return nil, nil, nil
	}
	err := f.errs[0]
	f.errs = f.errs[1:]
	return nil, nil, err
}

// applyAuditRows reads kind="recommendation_apply" rows from path.
func applyAuditRows(t *testing.T, path string) []audit.Entry {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	out := []audit.Entry{}
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		if line == "" {
			continue
		}
		var e audit.Entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("parse: %v", err)
		}
		if e.Kind == "recommendation_apply" {
			out = append(out, e)
		}
	}
	return out
}

func TestAutoApply_FormatGoOnSave_HappyPath(t *testing.T) {
	logger, path := newAuditLogger(t)
	ex := &fakeOSExec{}
	now := func() time.Time { return time.Unix(1700000000, 0).UTC() }
	app := NewAutoApplier(logger, now)
	if err := app.Register(FormatGoOnSave(ex, "/tmp/foo.go")); err != nil {
		t.Fatalf("register: %v", err)
	}

	rec := Recommendation{ID: "r1", Pattern: "format_go_on_save", Tier: TierAuto}
	if err := app.Apply(context.Background(), rec); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(ex.calls) != 1 || ex.calls[0].name != "gofmt" {
		t.Fatalf("want gofmt invocation, got %+v", ex.calls)
	}
	rows := applyAuditRows(t, path)
	if len(rows) != 1 || rows[0].Outcome != "success" || rows[0].Detail != "format_go_on_save" {
		t.Fatalf("want success audit row, got %+v", rows)
	}
}

func TestAutoApply_RateLimit_RejectsBurst(t *testing.T) {
	logger, _ := newAuditLogger(t)
	ex := &fakeOSExec{}
	now := func() time.Time { return time.Unix(1700000000, 0).UTC() }
	app := NewAutoApplier(logger, now)
	if err := app.Register(FormatGoOnSave(ex, "/tmp/foo.go")); err != nil {
		t.Fatalf("register: %v", err)
	}

	rec := Recommendation{ID: "r1", Pattern: "format_go_on_save", Tier: TierAuto}
	for i := 0; i < 10; i++ {
		if err := app.Apply(context.Background(), rec); err != nil {
			t.Fatalf("apply %d: %v", i, err)
		}
	}
	if err := app.Apply(context.Background(), rec); !errors.Is(err, ErrAutoLimitExceeded) {
		t.Fatalf("want ErrAutoLimitExceeded on 11th apply, got %v", err)
	}
}

func TestAutoApply_FailureAuditsThenSurfaces(t *testing.T) {
	logger, path := newAuditLogger(t)
	gofmtErr := errors.New("gofmt: syntax error")
	ex := &fakeOSExec{errs: []error{gofmtErr}}
	now := func() time.Time { return time.Unix(1700000000, 0).UTC() }
	app := NewAutoApplier(logger, now)
	if err := app.Register(FormatGoOnSave(ex, "/tmp/bad.go")); err != nil {
		t.Fatalf("register: %v", err)
	}

	rec := Recommendation{ID: "r1", Pattern: "format_go_on_save", Tier: TierAuto}
	err := app.Apply(context.Background(), rec)
	if !errors.Is(err, gofmtErr) {
		t.Fatalf("want gofmt err to surface, got %v", err)
	}
	rows := applyAuditRows(t, path)
	if len(rows) != 1 || rows[0].Outcome != "failed" {
		t.Fatalf("want failed audit row, got %+v", rows)
	}
}

func TestAutoApply_NotRegistered_NoOp(t *testing.T) {
	logger, _ := newAuditLogger(t)
	now := func() time.Time { return time.Unix(1700000000, 0).UTC() }
	app := NewAutoApplier(logger, now)

	rec := Recommendation{ID: "r1", Pattern: "unknown_pattern", Tier: TierAuto}
	if err := app.Apply(context.Background(), rec); !errors.Is(err, ErrPatternUnknown) {
		t.Fatalf("want ErrPatternUnknown, got %v", err)
	}
}

func TestAutoApply_CommitAtFocusEnd_HappyPath(t *testing.T) {
	logger, path := newAuditLogger(t)
	ex := &fakeOSExec{}
	now := func() time.Time { return time.Unix(1700000000, 0).UTC() }
	app := NewAutoApplier(logger, now)
	if err := app.Register(CommitAtFocusEnd(ex, "WIP: focus end")); err != nil {
		t.Fatalf("register: %v", err)
	}

	rec := Recommendation{ID: "r2", Pattern: "commit_at_focus_end", Tier: TierAuto}
	if err := app.Apply(context.Background(), rec); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(ex.calls) != 1 || ex.calls[0].name != "git" || ex.calls[0].args[0] != "commit" {
		t.Fatalf("want git commit, got %+v", ex.calls)
	}
	rows := applyAuditRows(t, path)
	if len(rows) != 1 || rows[0].Outcome != "success" {
		t.Fatalf("want success audit row, got %+v", rows)
	}
}
