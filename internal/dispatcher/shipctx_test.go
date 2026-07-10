package dispatcher

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeExec records calls + returns canned stdout per arg-prefix match.
type fakeExec struct {
	calls    [][]string
	respFor  map[string]string // first 3 args joined w/ space → stdout
	errOnArg string            // if non-empty, any call w/ this arg returns error
}

func (f *fakeExec) Run(_ context.Context, args []string) (string, error) {
	f.calls = append(f.calls, args)
	if f.errOnArg != "" {
		for _, a := range args {
			if a == f.errOnArg {
				return "", &execErr{msg: "synthetic " + f.errOnArg + " fail"}
			}
		}
	}
	// Key is first 3 args (e.g. "gh pr view") — keeps tests legible.
	key := strings.Join(args[:3], " ")
	if v, ok := f.respFor[key]; ok {
		return v, nil
	}
	return "", &execErr{msg: "no fixture for " + key}
}

type execErr struct{ msg string }

func (e *execErr) Error() string { return e.msg }

func TestFetchPRContext_ReturnsFormatted(t *testing.T) {
	fe := &fakeExec{respFor: map[string]string{
		"gh pr view": `{"title":"Add foo","body":"PR body line1\nline2","headRefName":"feat/foo"}`,
		"gh pr diff": "diff --git a/x b/x\n+added\n",
	}}
	got, err := FetchPRContext(context.Background(), fe, "trilam/leah", 42)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	for _, want := range []string{"PR #42", "trilam/leah", "Add foo", "feat/foo", "PR body line1", "diff --git", "+added"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestFetchPRContext_DiffFailureIsNonFatal(t *testing.T) {
	fe := &fakeExec{
		respFor: map[string]string{
			"gh pr view": `{"title":"x","body":"y","headRefName":"z"}`,
		},
		errOnArg: "diff",
	}
	got, err := FetchPRContext(context.Background(), fe, "r/r", 1)
	if err != nil {
		t.Fatalf("view should succeed even if diff fails: %v", err)
	}
	if !strings.Contains(got, "(diff fetch failed") {
		t.Errorf("expected diff-fail marker in:\n%s", got)
	}
}

func TestFetchPRContext_TruncatesLongDiff(t *testing.T) {
	var big strings.Builder
	for i := 0; i < prDiffMaxLines+50; i++ {
		big.WriteString("line\n")
	}
	fe := &fakeExec{respFor: map[string]string{
		"gh pr view": `{"title":"t","body":"b","headRefName":"h"}`,
		"gh pr diff": big.String(),
	}}
	got, _ := FetchPRContext(context.Background(), fe, "r/r", 1)
	if !strings.Contains(got, "... (truncated)") {
		t.Error("expected truncation marker")
	}
}

func TestFetchIssueContext_ReturnsFormatted(t *testing.T) {
	fe := &fakeExec{respFor: map[string]string{
		"gh issue view": `{"title":"Bug","body":"repro: ...","comments":[{"author":{"login":"alice"},"body":"me too"},{"author":{"login":"bob"},"body":"fixed in #99"}]}`,
	}}
	got, err := FetchIssueContext(context.Background(), fe, "trilam/leah", 7)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	for _, want := range []string{"issue #7", "Bug", "repro:", "@alice", "@bob", "fixed in #99", "Comments (2)"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestFetchIssueContext_GhError(t *testing.T) {
	fe := &fakeExec{errOnArg: "view"}
	_, err := FetchIssueContext(context.Background(), fe, "r/r", 1)
	if err == nil {
		t.Fatal("expected error on gh failure")
	}
}

func TestFetchThreadContext_LastNCommands(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".zsh_history")
	body := ""
	for i := 1; i <= 20; i++ {
		body += "cmd" + itoa(i) + "\n"
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := FetchThreadContext("5c", path)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(got, "cmd20") || !strings.Contains(got, "cmd16") {
		t.Errorf("expected last-5 commands (cmd16..cmd20) in:\n%s", got)
	}
	if strings.Contains(got, "cmd15") {
		t.Errorf("cmd15 should have been excluded:\n%s", got)
	}
}

func TestFetchThreadContext_StripsZshTimestamps(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".zsh_history")
	body := ": 1717000000:0;ls -la\n: 1717000001:0;git status\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	got, _ := FetchThreadContext("2c", path)
	if !strings.Contains(got, "ls -la") || !strings.Contains(got, "git status") {
		t.Errorf("expected stripped commands in:\n%s", got)
	}
	if strings.Contains(got, "1717000000") {
		t.Errorf("timestamp should be stripped:\n%s", got)
	}
}

func TestFetchThreadContext_InvalidWindow(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "h")
	_ = os.WriteFile(path, []byte("x\n"), 0o600)
	_, err := FetchThreadContext("abc", path)
	if err == nil {
		t.Error("expected error on invalid window")
	}
}

func TestFetchThreadContext_EmptyPathSkips(t *testing.T) {
	got, err := FetchThreadContext("5c", "")
	if err != nil {
		t.Fatalf("empty path should not error, got: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty result, got: %q", got)
	}
}

func TestComposeContext_PrependsAllNonEmpty(t *testing.T) {
	got := ComposeContext("PR_BLOCK", "ISSUE_BLOCK", "THREAD_BLOCK")
	if !strings.Contains(got, "PR_BLOCK") || !strings.Contains(got, "ISSUE_BLOCK") || !strings.Contains(got, "THREAD_BLOCK") {
		t.Errorf("missing blocks: %q", got)
	}
	if !strings.HasSuffix(got, "\n\n") {
		t.Error("expected trailing blank line")
	}
}

func TestComposeContext_SkipsEmpty(t *testing.T) {
	got := ComposeContext("", "ISSUE_ONLY", "")
	if got != "ISSUE_ONLY\n\n" {
		t.Errorf("unexpected compose result: %q", got)
	}
}

func TestComposeContext_AllEmpty(t *testing.T) {
	if got := ComposeContext("", "", ""); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

// itoa is a tiny test-local int→str helper (avoids strconv import).
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [16]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}
