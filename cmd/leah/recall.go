package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/budget"
	"github.com/trilam/leah/internal/embed"
	"github.com/trilam/leah/internal/reasoner"
)

// recallResult is one hit from Tier 1 search. Source is a short tag
// ("audit"|"contact"|"project"|"decision"); Timestamp is RFC3339 when
// known (else empty); Text is the human-readable summary line.
type recallResult struct {
	Source    string
	Timestamp string
	Text      string
}

// runRecall implements `leah recall [--llm] [--semantic] <query>`.
//
// Tier 1 (default): substring grep across audit.jsonl (last 30d) +
// LIKE %q% over contact/project/decision rows.
// Tier 1.5 (--semantic): cosine search over the `embedding` table
// (schema v5). Backend picked by LEAH_EMBED_BACKEND env (hash | openai).
// Tier 2 (--llm): pass hits to the Reasoner for a single synthesis call
// (budget-gated). --semantic and --llm compose: semantic feeds the LLM.
func runRecall(ctx context.Context, args []string) int {
	if shouldShowHelp(args) {
		_, _ = fmt.Fprintln(os.Stderr, "usage: leah recall [--llm] [--semantic] [--since=<RFC3339>] <query>")
		return 0
	}
	since, args := parseSinceFlag(args)
	var sinceT time.Time
	if since != "" {
		t, err := time.Parse(time.RFC3339, since)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "leah recall: invalid --since %q: %v\n", since, err)
			return 2
		}
		sinceT = t
	}
	useLLM := false
	useSemantic := false
	rest := make([]string, 0, len(args))
	for _, a := range args {
		if a == "--llm" {
			useLLM = true
			continue
		}
		if a == "--semantic" {
			useSemantic = true
			continue
		}
		rest = append(rest, a)
	}
	query := strings.TrimSpace(strings.Join(rest, " "))
	if query == "" {
		_, _ = fmt.Fprintln(os.Stderr, "usage: leah recall [--llm] [--semantic] [--since=<RFC3339>] <query>")
		return 2
	}

	a := &audit.Logger{Path: filepath.Join(stateDir(), "audit.jsonl")}

	store, err := openMemoryStore()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	defer func() { _ = store.Close() }()

	var results []recallResult
	if useSemantic {
		semHits, err := semanticRecall(ctx, store.DB(), query)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "leah recall: semantic: %v\n", err)
			return 1
		}
		results = semHits
	} else {
		auditHits, err := grepAudit(a.Path, query, 30*24*time.Hour, time.Now())
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "leah recall: audit scan: %v\n", err)
			return 1
		}
		memHits, err := grepMemory(store.DB(), query)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "leah recall: memory scan: %v\n", err)
			return 1
		}
		results = append(auditHits, memHits...)
		sort.SliceStable(results, func(i, j int) bool {
			return results[i].Timestamp > results[j].Timestamp
		})
	}

	if since != "" {
		results = filterResultsSince(results, sinceT)
	}

	if len(results) == 0 {
		_, _ = fmt.Println("no matches")
		_ = a.Append(audit.Entry{
			Kind:        "recall",
			ArgsHash:    query,
			BlastRadius: 0,
			Outcome:     "success",
			Detail:      "matches=0",
		})
		return 0
	}

	if !useLLM {
		for _, r := range results {
			printResult(os.Stdout, r)
		}
		_ = a.Append(audit.Entry{
			Kind:        "recall",
			ArgsHash:    query,
			BlastRadius: 0,
			Outcome:     "success",
			Detail:      fmt.Sprintf("matches=%d", len(results)),
		})
		return 0
	}

	// Tier 2: LLM synthesis.
	b := budget.New()
	client, err := reasoner.NewAnthropicClient()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah recall: %v\n", err)
		return 1
	}
	sys := "You are Leah's recall synthesizer. Given audit + memory hits, " +
		"summarize in 3-5 sentences what the operator was doing related to the query. " +
		"Lead with the most load-bearing finding. Cite source tags (audit/contact/project/decision). " +
		"Be terse. No preamble."
	r := &reasoner.Reasoner{Client: client, Budget: b, SystemPrompt: sys}

	if err := streamRecallSynthesis(ctx, r, os.Stdout, query, results); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah recall: %v\n", err)
		_ = a.Append(audit.Entry{
			Kind:        "recall",
			ArgsHash:    query,
			BlastRadius: 0,
			Outcome:     "failed",
			CostDollars: b.Spent(),
			Detail:      err.Error(),
		})
		return 1
	}
	_ = a.Append(audit.Entry{
		Kind:        "recall",
		ArgsHash:    query,
		BlastRadius: 0,
		Outcome:     "success",
		CostDollars: b.Spent(),
		Detail:      fmt.Sprintf("matches=%d llm=1", len(results)),
	})
	return 0
}

// recallStreamReasoner is the AskStream-only slice of *reasoner.Reasoner the
// --llm path needs. Narrow surface lets the test drive a scripted fake.
type recallStreamReasoner interface {
	AskStream(ctx context.Context, user string) (<-chan string, error)
}

// streamRecallSynthesis builds the Tier-2 synthesis prompt and streams the
// reasoner reply straight to w. WHY: mirrors `suggest --llm` so operators see
// first-token latency on recall too, instead of staring at a blocked Ask.
func streamRecallSynthesis(ctx context.Context, sr recallStreamReasoner, w io.Writer, query string, results []recallResult) error {
	var sb strings.Builder
	_, _ = fmt.Fprintf(&sb, "Query: %s\n\nMatches:\n", query)
	for _, hit := range results {
		_, _ = fmt.Fprintf(&sb, "- [%s %s] %s\n", hit.Source, hit.Timestamp, hit.Text)
	}
	ch, err := sr.AskStream(ctx, sb.String())
	if err != nil {
		return err
	}
	for delta := range ch {
		if _, werr := io.WriteString(w, delta); werr != nil {
			return werr
		}
	}
	_, werr := io.WriteString(w, "\n")
	return werr
}

// grepAudit scans the audit JSONL for case-insensitive substring matches
// in kind/detail/args_hash within the last `window` from `now`. Missing
// file is treated as "no hits" (first-run UX, not an error).
func grepAudit(path, query string, window time.Duration, now time.Time) ([]recallResult, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("open audit: %w", err)
	}
	defer func() { _ = f.Close() }()

	cutoff := now.Add(-window)
	needle := strings.ToLower(query)

	var out []recallResult
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var e audit.Entry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			continue
		}
		ts, err := time.Parse(time.RFC3339, e.Timestamp)
		if err == nil && ts.Before(cutoff) {
			continue
		}
		hay := strings.ToLower(e.Kind + " " + e.Detail + " " + e.ArgsHash)
		if !strings.Contains(hay, needle) {
			continue
		}
		text := e.Kind
		if e.Detail != "" {
			text = e.Kind + ": " + e.Detail
		}
		out = append(out, recallResult{
			Source:    "audit",
			Timestamp: e.Timestamp,
			Text:      text,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan audit: %w", err)
	}
	return out, nil
}

// grepMemory runs LIKE %q% against contact/project/decision in the
// default workspace. Schema columns differ per table — see schema.sql.
func grepMemory(db *sql.DB, query string) ([]recallResult, error) {
	pattern := "%" + strings.ReplaceAll(strings.ReplaceAll(query, `\`, `\\`), `%`, `\%`) + "%"
	var out []recallResult

	contactRows, err := db.Query(
		`SELECT name, COALESCE(email,''), COALESCE(notes,''), updated_at
		 FROM contact
		 WHERE workspace_id='default'
		   AND (LOWER(name) LIKE LOWER(?) ESCAPE '\'
		     OR LOWER(COALESCE(email,'')) LIKE LOWER(?) ESCAPE '\'
		     OR LOWER(COALESCE(notes,'')) LIKE LOWER(?) ESCAPE '\')`,
		pattern, pattern, pattern,
	)
	if err != nil {
		return nil, fmt.Errorf("query contact: %w", err)
	}
	for contactRows.Next() {
		var name, email, notes, ts string
		if err := contactRows.Scan(&name, &email, &notes, &ts); err != nil {
			_ = contactRows.Close()
			return nil, fmt.Errorf("scan contact: %w", err)
		}
		text := name
		if email != "" {
			text += " <" + email + ">"
		}
		if notes != "" {
			text += " — " + notes
		}
		out = append(out, recallResult{Source: "contact", Timestamp: ts, Text: text})
	}
	_ = contactRows.Close()

	projectRows, err := db.Query(
		`SELECT name, status, COALESCE(notes,''), updated_at
		 FROM project
		 WHERE workspace_id='default'
		   AND (LOWER(name) LIKE LOWER(?) ESCAPE '\'
		     OR LOWER(COALESCE(notes,'')) LIKE LOWER(?) ESCAPE '\')`,
		pattern, pattern,
	)
	if err != nil {
		return nil, fmt.Errorf("query project: %w", err)
	}
	for projectRows.Next() {
		var name, status, notes, ts string
		if err := projectRows.Scan(&name, &status, &notes, &ts); err != nil {
			_ = projectRows.Close()
			return nil, fmt.Errorf("scan project: %w", err)
		}
		text := fmt.Sprintf("[%s] %s", status, name)
		if notes != "" {
			text += " — " + notes
		}
		out = append(out, recallResult{Source: "project", Timestamp: ts, Text: text})
	}
	_ = projectRows.Close()

	decisionRows, err := db.Query(
		`SELECT topic, choice, COALESCE(rationale,''), decided_at
		 FROM decision
		 WHERE workspace_id='default'
		   AND (LOWER(topic) LIKE LOWER(?) ESCAPE '\'
		     OR LOWER(choice) LIKE LOWER(?) ESCAPE '\'
		     OR LOWER(COALESCE(rationale,'')) LIKE LOWER(?) ESCAPE '\')`,
		pattern, pattern, pattern,
	)
	if err != nil {
		return nil, fmt.Errorf("query decision: %w", err)
	}
	for decisionRows.Next() {
		var topic, choice, rationale, ts string
		if err := decisionRows.Scan(&topic, &choice, &rationale, &ts); err != nil {
			_ = decisionRows.Close()
			return nil, fmt.Errorf("scan decision: %w", err)
		}
		text := topic + " → " + choice
		if rationale != "" {
			text += " (" + rationale + ")"
		}
		out = append(out, recallResult{Source: "decision", Timestamp: ts, Text: text})
	}
	_ = decisionRows.Close()

	return out, nil
}

func printResult(w *os.File, r recallResult) {
	if r.Timestamp == "" {
		_, _ = fmt.Fprintf(w, "[%s] %s\n", r.Source, r.Text)
		return
	}
	_, _ = fmt.Fprintf(w, "[%s %s] %s\n", r.Source, r.Timestamp, r.Text)
}

// semanticRecall runs cosine search over the `embedding` table using the
// generator chosen by LEAH_EMBED_BACKEND (default: hash, offline). Returns
// rows in score-descending order, mapped into the existing recallResult
// shape so the LLM-synthesis path downstream stays oblivious to backend.
//
// First-run UX: if no embeddings have been ingested yet (semantic index
// empty), this returns zero results rather than an error. Embedding-ingest
// itself lives in a separate `leah ingest` command tracked as a followup;
// for now the operator seeds the index by running their existing capture
// flows after schema v5 lands (Put hooks added per-table in a successor PR).
func semanticRecall(ctx context.Context, db *sql.DB, query string) ([]recallResult, error) {
	gen, err := embed.SelectGenerator()
	if err != nil {
		return nil, err
	}
	store := embed.NewStore(db, gen)
	hits, err := store.Search(ctx, query, 10)
	if err != nil {
		return nil, err
	}
	out := make([]recallResult, 0, len(hits))
	for _, h := range hits {
		out = append(out, recallResult{
			Source:    h.Item.Type,
			Timestamp: "", // embedding rows carry updated_at, not semantic ts
			Text:      fmt.Sprintf("(sim=%.3f) %s", h.Score, h.Item.Content),
		})
	}
	return out, nil
}

// parseSinceFlag strips `--since=<v>` and `--since <v>` out of args, returning
// the value (empty if absent) and the remaining args. Mirrors the shape used by
// `leah suggest replay` so operators see one cursor syntax across surfaces.
func parseSinceFlag(args []string) (string, []string) {
	var since string
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case strings.HasPrefix(a, "--since="):
			since = a[len("--since="):]
		case a == "--since" && i+1 < len(args):
			since = args[i+1]
			i++
		default:
			rest = append(rest, a)
		}
	}
	return since, rest
}

// filterResultsSince keeps rows whose RFC3339 Timestamp is at or after the
// cutoff. Empty or unparseable timestamps pass through — semantic hits have no
// semantic ts (see semanticRecall) and dropping them would silently void the
// --semantic + --since combo.
func filterResultsSince(in []recallResult, cutoff time.Time) []recallResult {
	out := make([]recallResult, 0, len(in))
	for _, r := range in {
		if r.Timestamp == "" {
			out = append(out, r)
			continue
		}
		ts, err := time.Parse(time.RFC3339, r.Timestamp)
		if err != nil {
			out = append(out, r)
			continue
		}
		if ts.Before(cutoff) {
			continue
		}
		out = append(out, r)
	}
	return out
}
