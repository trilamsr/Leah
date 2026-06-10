package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAppendWritesOneLinePerEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	logger := &Logger{Path: path, Now: func() time.Time { return time.Unix(1700000000, 0).UTC() }}

	e1 := Entry{Kind: "ask", ArgsHash: "abc", BlastRadius: 1, Outcome: "success"}
	e2 := Entry{Kind: "ship", ArgsHash: "def", BlastRadius: 3, Outcome: "pending"}

	if err := logger.Append(e1); err != nil {
		t.Fatalf("append e1: %v", err)
	}
	if err := logger.Append(e2); err != nil {
		t.Fatalf("append e2: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	lines := splitLines(string(data))
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d: %q", len(lines), data)
	}

	var got1, got2 Entry
	if err := json.Unmarshal([]byte(lines[0]), &got1); err != nil {
		t.Fatalf("parse line 1: %v", err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &got2); err != nil {
		t.Fatalf("parse line 2: %v", err)
	}

	if got1.Kind != "ask" || got1.ArgsHash != "abc" || got1.BlastRadius != 1 || got1.Outcome != "success" {
		t.Errorf("line 1 mismatch: %+v", got1)
	}
	if got2.Kind != "ship" || got2.ArgsHash != "def" || got2.BlastRadius != 3 || got2.Outcome != "pending" {
		t.Errorf("line 2 mismatch: %+v", got2)
	}
	if got1.Timestamp != "2023-11-14T22:13:20Z" {
		t.Errorf("line 1 timestamp wrong: %q", got1.Timestamp)
	}
}

func TestAppendAppendsNotTruncates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	logger := &Logger{Path: path, Now: func() time.Time { return time.Unix(1700000000, 0).UTC() }}

	for i := 0; i < 10; i++ {
		if err := logger.Append(Entry{Kind: "ask", ArgsHash: "x"}); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}

	data, _ := os.ReadFile(path)
	if got := len(splitLines(string(data))); got != 10 {
		t.Fatalf("want 10 lines, got %d", got)
	}
}

// TestAppendPreservesWorkspace asserts the new Workspace field roundtrips
// through JSON so downstream readers (status, retro, patterns) can filter
// audit rows by active workspace. Backward-compat: omitempty keeps legacy
// rows valid JSON.
func TestAppendPreservesWorkspace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	logger := &Logger{Path: path, Now: func() time.Time { return time.Unix(1700000000, 0).UTC() }}

	if err := logger.Append(Entry{Kind: "ask", Workspace: "acme", Outcome: "success"}); err != nil {
		t.Fatalf("append tagged: %v", err)
	}
	if err := logger.Append(Entry{Kind: "ask", Outcome: "success"}); err != nil {
		t.Fatalf("append untagged: %v", err)
	}

	data, _ := os.ReadFile(path)
	lines := splitLines(string(data))
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d", len(lines))
	}

	var tagged, untagged Entry
	if err := json.Unmarshal([]byte(lines[0]), &tagged); err != nil {
		t.Fatalf("parse tagged: %v", err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &untagged); err != nil {
		t.Fatalf("parse untagged: %v", err)
	}
	if tagged.Workspace != "acme" {
		t.Errorf("Workspace lost: %q", tagged.Workspace)
	}
	if untagged.Workspace != "" {
		t.Errorf("untagged Workspace = %q, want empty", untagged.Workspace)
	}
	// Untagged row must not contain the "workspace" JSON key (omitempty).
	if !json.Valid([]byte(lines[1])) {
		t.Fatalf("line 2 not valid JSON: %s", lines[1])
	}
	if contains(lines[1], `"workspace"`) {
		t.Errorf("untagged row leaked workspace key: %s", lines[1])
	}
}

// TestAppendDefaultWorkspaceCallback asserts Logger.DefaultWorkspace stamps
// an unset Entry.Workspace at append time — the runAsk / runShip wiring
// uses this to tag every audit row with the active workspace without
// passing it through every callsite.
func TestAppendDefaultWorkspaceCallback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	logger := &Logger{
		Path:             path,
		Now:              func() time.Time { return time.Unix(1700000000, 0).UTC() },
		DefaultWorkspace: func() string { return "acme" },
	}

	// Unset Workspace → default fires.
	if err := logger.Append(Entry{Kind: "ask"}); err != nil {
		t.Fatalf("append: %v", err)
	}
	// Caller-set Workspace wins over default.
	if err := logger.Append(Entry{Kind: "ask", Workspace: "personal"}); err != nil {
		t.Fatalf("append explicit: %v", err)
	}

	data, _ := os.ReadFile(path)
	lines := splitLines(string(data))
	var defaulted, explicit Entry
	_ = json.Unmarshal([]byte(lines[0]), &defaulted)
	_ = json.Unmarshal([]byte(lines[1]), &explicit)
	if defaulted.Workspace != "acme" {
		t.Errorf("default not applied: %q", defaulted.Workspace)
	}
	if explicit.Workspace != "personal" {
		t.Errorf("explicit overridden: %q", explicit.Workspace)
	}
}

// TestEntry_NewFields_OmitEmptyOnZero asserts the seven W94 LLM-dim
// fields (Model, PromptSHA, InputTokens, OutputTokens, LatencyMS,
// EgressBytes, CacheHit) all carry omitempty so legacy rows stay
// byte-for-byte identical when callers leave them zero.
func TestEntry_NewFields_OmitEmptyOnZero(t *testing.T) {
	buf, err := json.Marshal(Entry{Kind: "ask", Outcome: "success"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(buf)
	for _, key := range []string{
		`"model"`, `"prompt_sha"`, `"input_tokens"`, `"output_tokens"`,
		`"latency_ms"`, `"egress_bytes"`, `"cache_hit"`,
	} {
		if contains(got, key) {
			t.Errorf("zero-value Entry leaked %s: %s", key, got)
		}
	}
}

// TestEntry_NewFields_RoundTrip asserts the seven new LLM-dim fields
// round-trip through JSON marshal/unmarshal so downstream readers
// (patterns, retro) get the values back unchanged.
func TestEntry_NewFields_RoundTrip(t *testing.T) {
	want := Entry{
		Kind:         "ask",
		Outcome:      "success",
		Model:        "claude-sonnet-4-6",
		PromptSHA:    "deadbeefcafef00d",
		InputTokens:  1234,
		OutputTokens: 567,
		LatencyMS:    890,
		EgressBytes:  2048,
		CacheHit:     true,
	}
	buf, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Entry
	if err := json.Unmarshal(buf, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got != want {
		t.Errorf("round-trip mismatch:\n got  %+v\n want %+v", got, want)
	}
}

// TestEntry_DecodesLegacyEntries asserts pre-W94 audit.jsonl rows
// (no model/prompt_sha/token/latency/egress/cache_hit keys) decode
// cleanly with zero values for the new fields — back-compat for
// patterns.Detect, selflearn.Resolver, operatormodel.Observe*.
func TestEntry_DecodesLegacyEntries(t *testing.T) {
	legacy := []byte(`{"ts":"2023-11-14T22:13:20Z","kind":"ask","args_hash":"abc","blast_radius":0,"outcome":"success","cost_dollars":0.012}`)
	var e Entry
	if err := json.Unmarshal(legacy, &e); err != nil {
		t.Fatalf("unmarshal legacy: %v", err)
	}
	if e.Kind != "ask" || e.ArgsHash != "abc" || e.Outcome != "success" || e.CostDollars != 0.012 {
		t.Errorf("legacy fields lost: %+v", e)
	}
	if e.Model != "" || e.PromptSHA != "" || e.InputTokens != 0 || e.OutputTokens != 0 ||
		e.LatencyMS != 0 || e.EgressBytes != 0 || e.CacheHit {
		t.Errorf("new fields not zero on legacy row: %+v", e)
	}
}

// TestAuditQuiesce_BlocksAppendDuringPass asserts QuiesceForConsolidation
// serializes Append calls; an Append issued mid-pass blocks until the
// release handle fires (§4 step 7 contract).
func TestAuditQuiesce_BlocksAppendDuringPass(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	logger := &Logger{Path: path, Now: func() time.Time { return time.Unix(1700000000, 0).UTC() }}

	release := logger.QuiesceForConsolidation()
	appended := make(chan error, 1)
	go func() {
		appended <- logger.Append(Entry{Kind: "post-quiesce", Outcome: "success"})
	}()

	select {
	case err := <-appended:
		t.Fatalf("Append returned during quiesce (err=%v): contract violated", err)
	case <-time.After(50 * time.Millisecond):
	}

	release()
	select {
	case err := <-appended:
		if err != nil {
			t.Fatalf("Append after release: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Append did not unblock after release")
	}

	data, _ := os.ReadFile(path)
	lines := splitLines(string(data))
	if len(lines) != 1 {
		t.Fatalf("want 1 post-release line, got %d", len(lines))
	}
}

// TestAuditQuiesce_NestedReleaseSafe asserts release is idempotent.
func TestAuditQuiesce_NestedReleaseSafe(t *testing.T) {
	dir := t.TempDir()
	logger := &Logger{Path: filepath.Join(dir, "audit.jsonl")}
	release := logger.QuiesceForConsolidation()
	release()
	release()
	if err := logger.Append(Entry{Kind: "post"}); err != nil {
		t.Fatalf("Append after double-release: %v", err)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func splitLines(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == '\n' {
			if cur != "" {
				out = append(out, cur)
			}
			cur = ""
		} else {
			cur += string(r)
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
