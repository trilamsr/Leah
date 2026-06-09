package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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

	profile, err := operatormodel.Load(ctx, store.DB())
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

