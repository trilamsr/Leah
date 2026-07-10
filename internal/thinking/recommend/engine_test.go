package recommend

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/trilam/leah/internal/platform/audit"
)

// auditContainsKind scans the audit JSONL for any row matching kind.
func auditContainsKind(t *testing.T, path, kind string) bool {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		if line == "" {
			continue
		}
		var e audit.Entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("parse audit row: %v", err)
		}
		if e.Kind == kind {
			return true
		}
	}
	return false
}

// newAuditLogger returns a tmp audit.jsonl + the Logger that targets it.
func newAuditLogger(t *testing.T) (*audit.Logger, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	return &audit.Logger{
		Path: path,
		Now:  func() time.Time { return time.Unix(1700000000, 0).UTC() },
	}, path
}

// TestEngine_Propose_EmptyByDefault verifies the skeleton returns no recs.
func TestEngine_Propose_EmptyByDefault(t *testing.T) {
	logger, _ := newAuditLogger(t)
	eng := NewMemoryEngine(logger)
	recs, err := eng.Propose(context.Background())
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	if len(recs) != 0 {
		t.Fatalf("want empty propose, got %d", len(recs))
	}
}

// TestEngine_Accept_LogsAuditRow verifies Accept writes recommendation_accept.
func TestEngine_Accept_LogsAuditRow(t *testing.T) {
	logger, path := newAuditLogger(t)
	eng := NewMemoryEngine(logger)
	rec := Recommendation{ID: "r1", Pattern: "commit_at_noon", Tier: TierConfirm, Source: "selflearn", Confidence: 0.8, CreatedAt: time.Now()}
	eng.Seed(rec)
	if err := eng.Accept(context.Background(), "r1"); err != nil {
		t.Fatalf("accept: %v", err)
	}
	if !auditContainsKind(t, path, "recommendation_accept") {
		t.Fatalf("audit log missing recommendation_accept row")
	}
}

// TestEngine_Reject_LogsAuditRow verifies Reject writes recommendation_reject.
func TestEngine_Reject_LogsAuditRow(t *testing.T) {
	logger, path := newAuditLogger(t)
	eng := NewMemoryEngine(logger)
	rec := Recommendation{ID: "r2", Pattern: "format_on_save", Tier: TierAuto, Source: "patterns", Confidence: 0.9, CreatedAt: time.Now()}
	eng.Seed(rec)
	if err := eng.Reject(context.Background(), "r2"); err != nil {
		t.Fatalf("reject: %v", err)
	}
	if !auditContainsKind(t, path, "recommendation_reject") {
		t.Fatalf("audit log missing recommendation_reject row")
	}
}

// TestEngine_Apply_AutoTier_OK verifies auto-tier rec executes Action.
func TestEngine_Apply_AutoTier_OK(t *testing.T) {
	logger, _ := newAuditLogger(t)
	eng := NewMemoryEngine(logger)
	called := false
	rec := Recommendation{
		ID: "r3", Pattern: "format_on_save", Tier: TierAuto,
		Source: "patterns", Confidence: 0.95, CreatedAt: time.Now(),
		Action: func(ctx context.Context) error { called = true; return nil },
	}
	if err := eng.Apply(context.Background(), rec); err != nil {
		t.Fatalf("apply auto: %v", err)
	}
	if !called {
		t.Fatalf("auto-tier Action not invoked")
	}
}

// TestEngine_Apply_ConfirmTier_RequiresPrior_Accept verifies confirm needs Accept first.
func TestEngine_Apply_ConfirmTier_RequiresPrior_Accept(t *testing.T) {
	logger, _ := newAuditLogger(t)
	eng := NewMemoryEngine(logger)
	rec := Recommendation{
		ID: "r4", Pattern: "commit_at_noon", Tier: TierConfirm,
		Source: "operatormodel", Confidence: 0.7, CreatedAt: time.Now(),
		Action: func(ctx context.Context) error { return nil },
	}
	eng.Seed(rec)

	if err := eng.Apply(context.Background(), rec); !errors.Is(err, ErrConfirmRequired) {
		t.Fatalf("want ErrConfirmRequired before Accept, got %v", err)
	}
	if err := eng.Accept(context.Background(), "r4"); err != nil {
		t.Fatalf("accept: %v", err)
	}
	if err := eng.Apply(context.Background(), rec); err != nil {
		t.Fatalf("apply after accept: %v", err)
	}
}

// TestEngine_Apply_BlockedTier_Rejects verifies blocked-tier never applies.
func TestEngine_Apply_BlockedTier_Rejects(t *testing.T) {
	logger, _ := newAuditLogger(t)
	eng := NewMemoryEngine(logger)
	rec := Recommendation{
		ID: "r5", Pattern: "mass_mail", Tier: TierBlocked,
		Source: "patterns", Confidence: 0.5, CreatedAt: time.Now(),
		Action: func(ctx context.Context) error { return nil },
	}
	if err := eng.Apply(context.Background(), rec); !errors.Is(err, ErrBlocked) {
		t.Fatalf("want ErrBlocked, got %v", err)
	}
}
