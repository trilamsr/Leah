package audit

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestTriage_FiltersAlarmingOnly asserts only failure-shaped rows survive.
func TestTriage_FiltersAlarmingOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	now := time.Now().UTC()
	lines := []string{
		`{"ts":"` + now.Format(time.RFC3339) + `","kind":"ask","outcome":"success"}`,
		`{"ts":"` + now.Format(time.RFC3339) + `","kind":"ship","outcome":"failed","detail":"diff empty"}`,
		`{"ts":"` + now.Format(time.RFC3339) + `","kind":"breaker.tripped","outcome":"ok","detail":"cost"}`,
		`{"ts":"` + now.Format(time.RFC3339) + `","kind":"daemon.transition","outcome":"observed","detail":"error: bus closed"}`,
		`{"ts":"` + now.Format(time.RFC3339) + `","kind":"daemon.transition","outcome":"observed","detail":"healthy"}`,
		`{"ts":"` + now.Format(time.RFC3339) + `","kind":"mail.emit_failure","outcome":"ok"}`,
		`{"ts":"` + now.Format(time.RFC3339) + `","kind":"dispatcher.draft.error","outcome":"ok"}`,
		`{"ts":"` + now.Format(time.RFC3339) + `","kind":"ask","outcome":"interrupted"}`,
	}
	if err := os.WriteFile(path, []byte(joinLines(lines)), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Triage(path, now.Add(-time.Hour), "")
	if err != nil {
		t.Fatalf("Triage: %v", err)
	}
	// Expect: failed, breaker.tripped, daemon.transition with error detail,
	// emit_failure, dispatcher.draft.error, interrupted = 6 rows.
	if len(got) != 6 {
		t.Fatalf("want 6 alarming rows, got %d: %+v", len(got), got)
	}
}

// TestTriage_SinceWindow drops rows older than cutoff.
func TestTriage_SinceWindow(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	old := time.Now().UTC().Add(-25 * time.Hour)
	fresh := time.Now().UTC().Add(-30 * time.Minute)
	lines := []string{
		`{"ts":"` + old.Format(time.RFC3339) + `","kind":"ship","outcome":"failed"}`,
		`{"ts":"` + fresh.Format(time.RFC3339) + `","kind":"ship","outcome":"failed"}`,
	}
	if err := os.WriteFile(path, []byte(joinLines(lines)), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Triage(path, time.Now().UTC().Add(-time.Hour), "")
	if err != nil {
		t.Fatalf("Triage: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 fresh row, got %d", len(got))
	}
}

// TestTriage_KindFilter restricts by substring match against the row's kind.
func TestTriage_KindFilter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	now := time.Now().UTC()
	lines := []string{
		`{"ts":"` + now.Format(time.RFC3339) + `","kind":"breaker.tripped","outcome":"ok"}`,
		`{"ts":"` + now.Format(time.RFC3339) + `","kind":"ship","outcome":"failed"}`,
		`{"ts":"` + now.Format(time.RFC3339) + `","kind":"mail.emit_failure","outcome":"ok"}`,
	}
	if err := os.WriteFile(path, []byte(joinLines(lines)), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Triage(path, now.Add(-time.Hour), "breaker")
	if err != nil {
		t.Fatalf("Triage: %v", err)
	}
	if len(got) != 1 || got[0].Kind != "breaker.tripped" {
		t.Fatalf("want only breaker.tripped, got %+v", got)
	}
}

// TestTriage_MissingFile returns empty + no error so fresh installs work.
func TestTriage_MissingFile(t *testing.T) {
	got, err := Triage(filepath.Join(t.TempDir(), "nope.jsonl"), time.Now(), "")
	if err != nil {
		t.Fatalf("Triage on missing: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("want 0, got %d", len(got))
	}
}

// TestIsAlarming_Cases pins the heuristic so the operator-visible signal set
// is the audit artifact reviewers check.
func TestIsAlarming_Cases(t *testing.T) {
	cases := []struct {
		name string
		e    Entry
		want bool
	}{
		{"success-ask", Entry{Kind: "ask", Outcome: "success"}, false},
		{"failed-ship", Entry{Kind: "ship", Outcome: "failed"}, true},
		{"interrupted-ask", Entry{Kind: "ask", Outcome: "interrupted"}, true},
		{"rejected-call", Entry{Kind: "call", Outcome: "rejected"}, true},
		{"breaker-tripped", Entry{Kind: "breaker.tripped", Outcome: "ok"}, true},
		{"daemon-transition-error", Entry{Kind: "daemon.transition", Outcome: "observed", Detail: "state=error"}, true},
		{"daemon-transition-healthy", Entry{Kind: "daemon.transition", Outcome: "observed", Detail: "state=healthy"}, false},
		{"emit-failure", Entry{Kind: "mail.emit_failure"}, true},
		{"dispatcher-error", Entry{Kind: "dispatcher.draft.error"}, true},
		{"auth-fail", Entry{Kind: "connect.auth_fail"}, true},
		{"audit-append-ok", Entry{Kind: "audit.append", Outcome: "ok"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsAlarming(c.e); got != c.want {
				t.Fatalf("IsAlarming(%+v) = %v, want %v", c.e, got, c.want)
			}
		})
	}
}

func joinLines(ls []string) string {
	out := ""
	for _, l := range ls {
		out += l + "\n"
	}
	return out
}
