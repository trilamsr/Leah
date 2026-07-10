package launcher

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeExec records the open(1) invocation so tests can assert on (name, args)
// without touching the host UI.
type fakeExec struct {
	called   int
	lastName string
	lastArgs []string
	err      error
}

func (f *fakeExec) Run(_ context.Context, name string, args []string, _ []byte) ([]byte, error) {
	f.called++
	f.lastName = name
	f.lastArgs = args
	return nil, f.err
}

func newTest(t *testing.T, ex ExecRunner) *Launcher {
	t.Helper()
	l, err := New(ex, defaultIntents())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return l
}

func TestLauncher_KnownIntent_ResolvesAndExecs(t *testing.T) {
	ex := &fakeExec{}
	l := newTest(t, ex)
	if err := l.Open(context.Background(), "netflix"); err != nil {
		t.Fatalf("Open netflix: %v", err)
	}
	if ex.called != 1 || ex.lastName != "open" {
		t.Fatalf("exec=%d name=%q, want 1/open", ex.called, ex.lastName)
	}
	if len(ex.lastArgs) != 1 || !strings.Contains(ex.lastArgs[0], "netflix.com") {
		t.Fatalf("args=%v, want netflix.com URL", ex.lastArgs)
	}
}

func TestLauncher_URLSchemePreferredOverWeb(t *testing.T) {
	ex := &fakeExec{}
	l := newTest(t, ex)
	if err := l.Open(context.Background(), "spotify"); err != nil {
		t.Fatalf("Open spotify: %v", err)
	}
	if len(ex.lastArgs) != 1 || !strings.HasPrefix(ex.lastArgs[0], "spotify:") {
		t.Fatalf("args=%v, want spotify: scheme", ex.lastArgs)
	}
}

func TestLauncher_UnknownIntent_ReturnsErr(t *testing.T) {
	ex := &fakeExec{}
	l := newTest(t, ex)
	err := l.Open(context.Background(), "nosuchapp")
	if !errors.Is(err, ErrUnknownTarget) {
		t.Fatalf("err=%v, want ErrUnknownTarget", err)
	}
	if ex.called != 0 {
		t.Fatalf("exec called %d times for unknown target", ex.called)
	}
}

func TestLauncher_AliasResolves(t *testing.T) {
	cases := []struct {
		alias   string
		wantSub string // substring expected in arg
	}{
		{"max", "max"},
		{"fb", "facebook.com"},
		{"ig", "instagram.com"},
		{"twitter", "x.com"},
		{"gh", "github.com"},
		{"primevideo", "primevideo.com"},
		{"disneyplus", "disneyplus.com"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.alias, func(t *testing.T) {
			ex := &fakeExec{}
			l := newTest(t, ex)
			if err := l.Open(context.Background(), tc.alias); err != nil {
				t.Fatalf("Open %q: %v", tc.alias, err)
			}
			if len(ex.lastArgs) != 1 || !strings.Contains(ex.lastArgs[0], tc.wantSub) {
				t.Fatalf("alias %q: args=%v, want substring %q", tc.alias, ex.lastArgs, tc.wantSub)
			}
		})
	}
}

func TestLauncher_NormalizesInput(t *testing.T) {
	cases := []string{"Netflix", "NETFLIX", "netflix", "  netflix  ", "Netflix\t"}
	for _, c := range cases {
		c := c
		t.Run(c, func(t *testing.T) {
			ex := &fakeExec{}
			l := newTest(t, ex)
			if err := l.Open(context.Background(), c); err != nil {
				t.Fatalf("Open %q: %v", c, err)
			}
			if len(ex.lastArgs) != 1 || !strings.Contains(ex.lastArgs[0], "netflix.com") {
				t.Fatalf("input %q: args=%v", c, ex.lastArgs)
			}
		})
	}
}

func TestLauncher_OpenFails_ReturnsErr(t *testing.T) {
	ex := &fakeExec{err: errors.New("boom")}
	l := newTest(t, ex)
	err := l.Open(context.Background(), "netflix")
	if err == nil {
		t.Fatal("Open: want error from failing exec")
	}
}

func TestNew_RejectsNilDeps(t *testing.T) {
	if _, err := New(nil, defaultIntents()); err == nil {
		t.Fatal("New(nil exec): want error")
	}
	if _, err := New(&fakeExec{}, nil); err == nil {
		t.Fatal("New(nil intents): want error")
	}
}
