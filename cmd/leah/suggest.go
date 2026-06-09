package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/trilam/leah/internal/ctxmgr"
	"github.com/trilam/leah/internal/memory"
	"github.com/trilam/leah/internal/operatormodel"
)

// runSuggest implements `leah suggest [--context X] [--llm]`. Surfaces the
// top-N recommendations from operator_profile for the current (ctx, time).
// Prints "not ready" when the cold-start gate (50 rows + 7 days) hasn't
// fired yet.
func runSuggest(args []string) {
	ctx := context.Background()

	activeContext := ""
	useLLM := false
	for i, a := range args {
		switch a {
		case "--context":
			if i+1 < len(args) {
				activeContext = args[i+1]
			}
		case "--llm":
			useLLM = true
		}
	}

	memPath := filepath.Join(stateDir(), "memory.db")
	store, err := memory.NewStore(memPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open memory: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = store.Close() }()

	if activeContext == "" {
		// ctxmgr opens its own *sql.DB on the same file; WAL mode + busy_timeout
		// permit co-existence with memory.Store.
		cm, err := ctxmgr.Open(memPath)
		if err == nil {
			defer func() { _ = cm.Close() }()
			if a, err := cm.Active(); err == nil {
				activeContext = a.Name
			}
		}
	}

	profile, err := loadProfile(ctx, store.DB())
	if err != nil {
		fmt.Fprintf(os.Stderr, "load profile: %v\n", err)
		os.Exit(1)
	}

	recs, err := operatormodel.Recommend(profile, activeContext, time.Now())
	if err != nil {
		fmt.Fprintf(os.Stderr, "recommend: %v\n", err)
		os.Exit(1)
	}
	if len(recs) == 0 {
		if profile.Ready {
			fmt.Println("no recommendations for current context + time")
		} else {
			fmt.Printf("operator-model not ready (have %d rows, %d days; need %d+ rows and %d+ days)\n",
				profile.RowsObserved, profile.DaysObserved,
				operatormodel.ColdStartMinRows, operatormodel.ColdStartMinDays)
		}
		return
	}

	if useLLM {
		fmt.Println("(--llm phrasing TODO; printing template form)")
	}
	for i, r := range recs {
		fmt.Printf("%d. %s — %s (weight %.2f)\n", i+1, r.Kind, r.Reason, r.Weight)
	}
}

// loadProfile reads the persisted operator_profile + operator_profile_meta
// tables (populated by operatormodel.UpdateProfile in the daemon weekly tick)
// and returns a Profile ready to feed operatormodel.Recommend.
//
// Inline reader (vs an operatormodel.Load helper) — operatormodel package is
// owned by a parallel agent in this wave; this CLI-side reader avoids touching
// it. Schema names mirror profile.go::persist.
func loadProfile(ctx context.Context, db *sql.DB) (operatormodel.Profile, error) {
	p := operatormodel.Profile{}
	meta := map[string]string{}
	mrows, err := db.QueryContext(ctx, `SELECT key, value FROM operator_profile_meta`)
	if err != nil {
		// Table may not exist yet on a fresh install — treat as not-ready.
		return p, nil
	}
	defer func() { _ = mrows.Close() }()
	for mrows.Next() {
		var k, v string
		if err := mrows.Scan(&k, &v); err != nil {
			return p, fmt.Errorf("scan meta: %w", err)
		}
		meta[k] = v
	}
	if n, err := strconv.Atoi(meta["rows_observed"]); err == nil {
		p.RowsObserved = n
	}
	if n, err := strconv.Atoi(meta["days_observed"]); err == nil {
		p.DaysObserved = n
	}
	p.Ready = p.RowsObserved >= operatormodel.ColdStartMinRows &&
		p.DaysObserved >= operatormodel.ColdStartMinDays

	rrows, err := db.QueryContext(ctx, `
		SELECT class, key, slot, count, weight, window_start, window_end
		FROM operator_profile`)
	if err != nil {
		return p, nil // empty profile == cold start
	}
	defer func() { _ = rrows.Close() }()
	for rrows.Next() {
		var r operatormodel.ProfileRow
		var ws, we string
		if err := rrows.Scan(&r.Class, &r.Key, &r.Slot, &r.Count, &r.Weight, &ws, &we); err != nil {
			return p, fmt.Errorf("scan row: %w", err)
		}
		if t, err := time.Parse(time.RFC3339, ws); err == nil {
			r.WindowStart = t
		}
		if t, err := time.Parse(time.RFC3339, we); err == nil {
			r.WindowEnd = t
		}
		p.Rows = append(p.Rows, r)
	}
	return p, nil
}
