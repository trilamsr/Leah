package activeapp

import (
	"context"
	"errors"
	"testing"
)

type fakeAttestor struct {
	err   error
	calls int
}

func (f *fakeAttestor) Attest(_ context.Context, _ string) error {
	f.calls++
	return f.err
}

type fakeExec struct {
	stdout []byte
	stderr []byte
	err    error
	calls  int
	last   struct {
		name string
		args []string
	}
}

func (f *fakeExec) Run(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
	f.calls++
	f.last.name = name
	f.last.args = args
	return f.stdout, f.stderr, f.err
}

func TestActiveApp_Name(t *testing.T) {
	a, err := New(Config{Attestor: &fakeAttestor{}, Exec: &fakeExec{}})
	if err != nil {
		t.Fatal(err)
	}
	if got := a.Name(); got != "ActiveApp" {
		t.Fatalf("Name=%q want ActiveApp", got)
	}
}

func TestActiveApp_New_RequiresDeps(t *testing.T) {
	if _, err := New(Config{Exec: &fakeExec{}}); err == nil {
		t.Fatal("want error on missing Attestor")
	}
	if _, err := New(Config{Attestor: &fakeAttestor{}}); err == nil {
		t.Fatal("want error on missing Exec")
	}
}

func TestActiveApp_Available_True(t *testing.T) {
	a, _ := New(Config{Attestor: &fakeAttestor{}, Exec: &fakeExec{}})
	if !a.Available(context.Background()) {
		t.Fatal("Available=false; want true")
	}
}

func TestActiveApp_Available_False(t *testing.T) {
	a, _ := New(Config{Attestor: &fakeAttestor{}, Exec: &fakeExec{err: errors.New("no osascript")}})
	if a.Available(context.Background()) {
		t.Fatal("Available=true; want false on exec error")
	}
}

// Denial must short-circuit BEFORE osascript runs — even the foreground-app
// signal is sensitive (spec §7 row: "Leah knows when I open Banking app").
func TestActiveApp_Query_AttestationDenied_NoExec(t *testing.T) {
	att := &fakeAttestor{err: errors.New("no")}
	ex := &fakeExec{}
	a, _ := New(Config{Attestor: att, Exec: ex})
	_, err := a.Query(context.Background())
	if !errors.Is(err, ErrAttestationDenied) {
		t.Fatalf("err=%v want ErrAttestationDenied", err)
	}
	if ex.calls != 0 {
		t.Fatalf("exec ran %d times after denial", ex.calls)
	}
}

func TestActiveApp_Query_HappyPath(t *testing.T) {
	ex := &fakeExec{stdout: []byte("Safari|com.apple.Safari\n")}
	a, _ := New(Config{Attestor: &fakeAttestor{}, Exec: ex})
	item, err := a.Query(context.Background())
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if item.Name != "Safari" || item.BundleID != "com.apple.Safari" {
		t.Fatalf("item=%+v", item)
	}
	if item.Redacted {
		t.Fatal("Redacted=true; want false")
	}
	if ex.last.name != "osascript" || len(ex.last.args) != 2 || ex.last.args[0] != "-e" {
		t.Fatalf("exec=%+v want osascript -e <script>", ex.last)
	}
}

// Spec §7: blocked bundle ID surfaces as redacted, not the real name.
func TestActiveApp_Query_BlocklistRedacts(t *testing.T) {
	ex := &fakeExec{stdout: []byte("Bank of X|com.bank.x.mac\n")}
	a, _ := New(Config{
		Attestor:  &fakeAttestor{},
		Exec:      ex,
		Blocklist: []string{"com.bank."},
	})
	item, err := a.Query(context.Background())
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if !item.Redacted {
		t.Fatal("Redacted=false; want true")
	}
	if item.Name != "(redacted)" || item.BundleID != "(redacted)" {
		t.Fatalf("item=%+v want redacted labels", item)
	}
}

func TestActiveApp_Query_BlocklistPrefix(t *testing.T) {
	tests := []struct {
		name    string
		bundle  string
		block   []string
		blocked bool
	}{
		{"exact match", "com.apple.Keychain", []string{"com.apple.Keychain"}, true},
		{"prefix match", "com.apple.KeychainAccess", []string{"com.apple.Keychain"}, true},
		{"no match", "com.apple.Safari", []string{"com.apple.Keychain"}, false},
		{"empty entry ignored", "com.apple.Safari", []string{""}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ex := &fakeExec{stdout: []byte("App|" + tc.bundle)}
			a, _ := New(Config{Attestor: &fakeAttestor{}, Exec: ex, Blocklist: tc.block})
			item, err := a.Query(context.Background())
			if err != nil {
				t.Fatalf("Query: %v", err)
			}
			if item.Redacted != tc.blocked {
				t.Fatalf("Redacted=%v want %v", item.Redacted, tc.blocked)
			}
		})
	}
}

func TestActiveApp_Query_ExecError(t *testing.T) {
	ex := &fakeExec{err: errors.New("boom")}
	a, _ := New(Config{Attestor: &fakeAttestor{}, Exec: ex})
	_, err := a.Query(context.Background())
	if !errors.Is(err, ErrSourceUnavailable) {
		t.Fatalf("err=%v want ErrSourceUnavailable", err)
	}
}

func TestActiveApp_Query_ParseFailure(t *testing.T) {
	cases := []string{
		"",
		"no-pipe-here",
		"|com.apple.Safari",
		"Safari|",
	}
	for _, out := range cases {
		t.Run(out, func(t *testing.T) {
			ex := &fakeExec{stdout: []byte(out)}
			a, _ := New(Config{Attestor: &fakeAttestor{}, Exec: ex})
			_, err := a.Query(context.Background())
			if !errors.Is(err, ErrParseFailed) {
				t.Fatalf("err=%v want ErrParseFailed", err)
			}
		})
	}
}

// App display names containing `|` (legal per System Events) used to truncate
// the bundle ID — locking the LastIndex split in.
func TestActiveApp_Query_NameContainsPipe(t *testing.T) {
	ex := &fakeExec{stdout: []byte("Foo | Bar|com.example.foobar\n")}
	a, _ := New(Config{Attestor: &fakeAttestor{}, Exec: ex})
	item, err := a.Query(context.Background())
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if item.Name != "Foo | Bar" {
		t.Fatalf("Name=%q want %q", item.Name, "Foo | Bar")
	}
	if item.BundleID != "com.example.foobar" {
		t.Fatalf("BundleID=%q want com.example.foobar", item.BundleID)
	}
}

// TCC denials must surface as ErrPermissionDenied so callers route to the
// permissions-help prompt instead of the generic source-unavailable retry.
func TestActiveApp_Query_PermissionDenied(t *testing.T) {
	ex := &fakeExec{stderr: []byte("execution error: Not authorized to send Apple events (-1743)"), err: errors.New("exit 1")}
	a, _ := New(Config{Attestor: &fakeAttestor{}, Exec: ex})
	_, err := a.Query(context.Background())
	if !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("err=%v want ErrPermissionDenied", err)
	}
}
