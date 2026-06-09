package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/budget"
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

// runRecall implements `leah recall [--llm] <query>`.
//
// Tier 1 (default): substring grep across audit.jsonl (last 30d) +
// LIKE %q% over contact/project/decision rows.
// Tier 2 (--llm): pass Tier-1 hits to the Reasoner for a single
// synthesis call (budget-gated).
func runRecall(args []string) {
	useLLM := false
	rest := make([]string, 0, len(args))
	for _, a := range args {
		if a == "--llm" {
			useLLM = true
			continue
		}
		rest = append(rest, a)
	}
	query := strings.TrimSpace(strings.Join(rest, " "))
	if query == "" {
		fmt.Fprintln(os.Stderr, "usage: leah recall [--llm] <query>")
		os.Exit(2)
	}

	a := &audit.Logger{Path: filepath.Join(stateDir(), "audit.jsonl")}

	store := openMemoryStore()
	defer func() { _ = store.Close() }()

	auditHits, err := grepAudit(a.Path, query, 30*24*time.Hour, time.Now())
	if err != nil {
		fmt.Fprintf(os.Stderr, "leah recall: audit scan: %v\n", err)
		os.Exit(1)
	}
	memHits, err := grepMemory(store.DB(), query)
	if err != nil {
		fmt.Fprintf(os.Stderr, "leah recall: memory scan: %v\n", err)
		os.Exit(1)
	}
	results := append(auditHits, memHits...)
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].Timestamp > results[j].Timestamp
	})

	if len(results) == 0 {
		fmt.Println("no matches")
		_ = a.Append(audit.Entry{
			Kind:        "recall",
			ArgsHash:    query,
			BlastRadius: 0,
			Outcome:     "success",
			Detail:      "matches=0",
		})
		return
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
		return
	}

	// Tier 2: LLM synthesis.
	b := budget.New()
	client, err := reasoner.NewAnthropicClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "leah recall: %v\n", err)
		os.Exit(1)
	}
	sys := "You are Leah's recall synthesizer. Given audit + memory hits, " +
		"summarize in 3-5 sentences what the operator was doing related to the query. " +
		"Lead with the most load-bearing finding. Cite source tags (audit/contact/project/decision). " +
		"Be terse. No preamble."
	r := &reasoner.Reasoner{Client: client, Budget: b, SystemPrompt: sys}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Query: %s\n\nMatches:\n", query)
	for _, hit := range results {
		fmt.Fprintf(&sb, "- [%s %s] %s\n", hit.Source, hit.Timestamp, hit.Text)
	}

	text, err := r.Ask(context.Background(), sb.String())
	if err != nil {
		fmt.Fprintf(os.Stderr, "leah recall: %v\n", err)
		_ = a.Append(audit.Entry{
			Kind:        "recall",
			ArgsHash:    query,
			BlastRadius: 0,
			Outcome:     "failed",
			CostDollars: b.Spent(),
			Detail:      err.Error(),
		})
		os.Exit(1)
	}
	fmt.Println(text)
	_ = a.Append(audit.Entry{
		Kind:        "recall",
		ArgsHash:    query,
		BlastRadius: 0,
		Outcome:     "success",
		CostDollars: b.Spent(),
		Detail:      fmt.Sprintf("matches=%d llm=1", len(results)),
	})
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
