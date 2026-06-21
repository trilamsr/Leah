package main

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/operatormodel"
)

// TestWeeklyFeedbackEdge_OutcomeReachesProfile runs the real resolver +
// operatormodel weekly tasks against a shared FeedbackObserver and asserts a
// resolved ship verdict lands as a ship_outcome row in operator_profile —
// producer-verifying the one-way feedback wiring fires end to end. A
// regatta.ship row with no PR URL resolves to "unknown" (dangling) without
// shelling out to gh.
func TestWeeklyFeedbackEdge_OutcomeReachesProfile(t *testing.T) {
	sd := t.TempDir()
	auditPath := filepath.Join(sd, "audit.jsonl")
	a := &audit.Logger{Path: auditPath}
	for _, h := range []string{"h1", "h2", "h3"} {
		if err := a.Append(audit.Entry{Kind: "regatta.ship", ArgsHash: h, Outcome: "pending", Detail: "no-url"}); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	feedback := &operatormodel.FeedbackObserver{}
	buildResolverTask(auditPath, a, nil, feedback)(context.Background())
	buildOperatorModelTask(sd, auditPath, nil, feedback)(context.Background())

	db, err := sql.Open("sqlite", filepath.Join(sd, "memory.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()
	var n int
	if err := db.QueryRow(
		`SELECT count(*) FROM operator_profile WHERE class='ship_outcome' AND slot=?`,
		operatormodel.VerdictDangling,
	).Scan(&n); err != nil {
		t.Fatalf("query: %v", err)
	}
	if n == 0 {
		t.Fatal("ship_outcome dangling row never reached operator_profile — feedback edge broken")
	}
}
