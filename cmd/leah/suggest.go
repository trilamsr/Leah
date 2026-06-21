package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/trilam/leah/internal/budget"
	"github.com/trilam/leah/internal/ctxmgr"
	"github.com/trilam/leah/internal/memory"
	"github.com/trilam/leah/internal/operatormodel"
	"github.com/trilam/leah/internal/reasoner"
)

// runSuggest implements `leah suggest [--context X] [--llm]`. Surfaces the
// top-N recommendations from operator_profile for the current (ctx, time).
// Prints "not ready" when the cold-start gate (50 rows + 7 days) hasn't
// fired yet.
//
// Sub-verbs: `leah suggest replay --since=<RFC3339>` enumerates the
// consolidated + raw rows since a point in time (S9 §7 — kept here vs a
// new top-level command to match the forget/quote per-verb pattern).
func runSuggest(ctx context.Context, args []string) int {
	if len(args) > 0 && args[0] == "replay" {
		return runSuggestReplay(ctx, args[1:])
	}
	if shouldShowHelp(args) {
		_, _ = fmt.Fprintln(os.Stderr, "usage: leah suggest [--context X] [--llm]")
		_, _ = fmt.Fprintln(os.Stderr, "       leah suggest replay --since=<RFC3339>")
		return 0
	}

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
		_, _ = fmt.Fprintf(os.Stderr, "open memory: %v\n", err)
		return 1
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
		_, _ = fmt.Fprintf(os.Stderr, "load profile: %v\n", err)
		return 1
	}

	recs, err := operatormodel.Recommend(profile, activeContext, time.Now())
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "recommend: %v\n", err)
		return 1
	}
	if len(recs) == 0 {
		if profile.Ready {
			_, _ = fmt.Println("no recommendations for current context + time")
		} else {
			fmt.Printf("operator-model not ready (have %d rows, %d days; need %d+ rows and %d+ days)\n",
				profile.RowsObserved, profile.DaysObserved,
				operatormodel.ColdStartMinRows, operatormodel.ColdStartMinDays)
		}
		return 0
	}

	if useLLM {
		sr, err := newSuggestReasoner()
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "suggest --llm: %v\n", err)
			return 1
		}
		if err := streamLLMPhrasing(ctx, sr, os.Stdout, recs); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "suggest --llm: %v\n", err)
			return 1
		}
		return 0
	}
	for i, r := range recs {
		fmt.Printf("%d. %s — %s (weight %.2f)\n", i+1, r.Kind, r.Reason, r.Weight)
	}
	return 0
}

// streamReasoner is the AskStream-only slice of *reasoner.Reasoner that the
// --llm phrasing path needs. Defined narrowly so the test can drive it with
// a scripted fake without spinning up the full reasoner stack.
type streamReasoner interface {
	AskStream(ctx context.Context, user string) (<-chan string, error)
}

// streamLLMPhrasing rephrases each template-form recommendation as one
// short natural-language sentence by streaming deltas straight to w.
// Each rec gets its own line; the trailing newline lands once the stream
// for that rec drains. WHY: the no-LLM path prints one line per rec, so
// the --llm path mirrors that shape — operators reading both side-by-side
// see the same row count.
func streamLLMPhrasing(ctx context.Context, sr streamReasoner, w io.Writer, recs []operatormodel.Recommendation) error {
	for _, r := range recs {
		prompt := fmt.Sprintf(
			"Rephrase this recommendation in one short natural-language sentence. "+
				"Don't add caveats or hedging. Template: %s — %s (weight %.2f)",
			r.Kind, r.Reason, r.Weight,
		)
		ch, err := sr.AskStream(ctx, prompt)
		if err != nil {
			return err
		}
		for delta := range ch {
			if _, werr := io.WriteString(w, delta); werr != nil {
				return werr
			}
		}
		if _, werr := io.WriteString(w, "\n"); werr != nil {
			return werr
		}
	}
	return nil
}

// newSuggestReasoner builds the production Reasoner the --llm path streams
// through. SystemPrompt + PersonaPrefix are both left empty on purpose: the
// phrasing prompt itself tells the model "no caveats or hedging," and any
// workspace persona prefix would re-introduce the very tone we're stripping.
func newSuggestReasoner() (*reasoner.Reasoner, error) {
	b := budget.New()
	br := openCostBreaker()
	client, err := reasoner.NewRoutedClient("reasoner", br, b, nil)
	if err != nil {
		return nil, err
	}
	return &reasoner.Reasoner{Client: client, Budget: b}, nil
}

// runSuggestReplay implements `leah suggest replay --since=<RFC3339>
// [--no-archive]`. Default: lists the durable consolidated rows whose
// source_window_end >= since AND the raw rows ≥ since in the archive
// jsonl so the operator can audit what the model summarized at any past
// instant without re-running the daemon pass. --no-archive suppresses the
// archive scan (DB only).
func runSuggestReplay(ctx context.Context, args []string) int {
	since := ""
	noArchive := false
	for i, a := range args {
		switch {
		case a == "--since" && i+1 < len(args):
			since = args[i+1]
		case strings.HasPrefix(a, "--since="):
			since = a[len("--since="):]
		case a == "--no-archive":
			noArchive = true
		}
	}
	if since == "" {
		_, _ = fmt.Fprintln(os.Stderr, "usage: leah suggest replay --since=<RFC3339> [--no-archive]")
		return 2
	}
	sinceT, err := time.Parse(time.RFC3339, since)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "replay: invalid --since %q: %v\n", since, err)
		return 2
	}

	memPath := filepath.Join(stateDir(), "memory.db")
	store, err := memory.NewStore(memPath)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "open memory: %v\n", err)
		return 1
	}
	defer func() { _ = store.Close() }()

	rows, err := store.DB().QueryContext(ctx, `
		SELECT class, key, slot, weight, count, first_seen_ts, last_consolidated_at, source_window_end
		FROM operator_profile_consolidated
		WHERE source_window_end >= ?
		ORDER BY source_window_end DESC`, since)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "replay query: %v\n", err)
		return 1
	}
	defer func() { _ = rows.Close() }()
	var printed int
	for rows.Next() {
		var class, key, slot, firstSeen, lastCons, srcEnd string
		var weight float64
		var count int
		if err := rows.Scan(&class, &key, &slot, &weight, &count, &firstSeen, &lastCons, &srcEnd); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "scan: %v\n", err)
			return 1
		}
		fmt.Printf("%s/%s/%s weight=%.2f count=%d window_end=%s\n",
			class, key, slot, weight, count, srcEnd)
		printed++
	}
	if noArchive {
		fmt.Printf("(%d consolidated rows since %s; archive skipped)\n", printed, since)
		return 0
	}
	archived := scanArchive(filepath.Join(stateDir(), "consolidated.jsonl"), sinceT)
	fmt.Printf("(%d consolidated rows + %d archive rows since %s)\n", printed, archived, since)
	return 0
}

// scanArchive streams the consolidated.jsonl archive and prints rows whose
// timestamp >= since. Missing archive = silent zero. Malformed lines are
// skipped (audit.jsonl reader posture).
func scanArchive(path string, since time.Time) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	var n int
	for sc.Scan() {
		var e struct {
			Timestamp string `json:"ts"`
			Kind      string `json:"kind"`
			Outcome   string `json:"outcome"`
		}
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			continue
		}
		ts, err := time.Parse(time.RFC3339, e.Timestamp)
		if err != nil {
			continue
		}
		if ts.Before(since) {
			continue
		}
		fmt.Printf("archive: %s kind=%s outcome=%s\n", e.Timestamp, e.Kind, e.Outcome)
		n++
	}
	return n
}

