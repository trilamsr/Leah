package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

// fakeResolver records calls and returns scripted provider names / err.
type fakeResolver struct {
	called    int
	lastQuery string
	names     []string
	err       error
}

func (f *fakeResolver) Resolve(_ context.Context, query string) ([]string, error) {
	f.called++
	f.lastQuery = query
	return f.names, f.err
}

func withFakeResolver(t *testing.T, r titleResolver) func() {
	t.Helper()
	prev := titleResolverFactory
	titleResolverFactory = func() (titleResolver, error) { return r, nil }
	return func() { titleResolverFactory = prev }
}

func withNoResolver(t *testing.T, sentinelErr error) func() {
	t.Helper()
	prev := titleResolverFactory
	titleResolverFactory = func() (titleResolver, error) { return nil, sentinelErr }
	return func() { titleResolverFactory = prev }
}

// Known intent must not consult TMDB at all.
func TestOpen_KnownIntent_BypassesTMDB(t *testing.T) {
	ex := &fakeOpenExec{}
	defer withFakeLauncher(t, ex)()
	res := &fakeResolver{names: []string{"should-not-be-used"}}
	defer withFakeResolver(t, res)()

	var buf bytes.Buffer
	code := runOpen(context.Background(), []string{"netflix"}, &buf)
	if code != 0 {
		t.Fatalf("exit=%d, want 0; out=%q", code, buf.String())
	}
	if res.called != 0 {
		t.Fatalf("resolver called %d times for known intent, want 0", res.called)
	}
	if ex.called != 1 {
		t.Fatalf("exec called %d, want 1", ex.called)
	}
}

// Unknown arg resolves to a title and routes via the first mappable flatrate.
func TestOpen_UnknownTitle_ResolvesViaTMDB_PicksFirstFlatrate(t *testing.T) {
	ex := &fakeOpenExec{}
	defer withFakeLauncher(t, ex)()
	res := &fakeResolver{names: []string{"HBO Max", "Hulu"}}
	defer withFakeResolver(t, res)()

	var buf bytes.Buffer
	code := runOpen(context.Background(), []string{"Game of Thrones"}, &buf)
	if code != 0 {
		t.Fatalf("exit=%d, want 0; out=%q", code, buf.String())
	}
	if res.called != 1 {
		t.Fatalf("resolver called %d, want 1", res.called)
	}
	if res.lastQuery != "Game of Thrones" {
		t.Fatalf("query=%q, want %q", res.lastQuery, "Game of Thrones")
	}
	if ex.called != 1 {
		t.Fatalf("exec called %d, want 1", ex.called)
	}
	// HBO Max → "max" → "https://play.max.com"
	if len(ex.lastArgs) != 1 || !strings.Contains(ex.lastArgs[0], "max.com") {
		t.Fatalf("launcher invoked with %v, want max.com URL", ex.lastArgs)
	}
}

// If the resolved provider doesn't map to a known intent key, surface options.
func TestOpen_TitleResolves_ProviderNotMapped_PrintsList(t *testing.T) {
	ex := &fakeOpenExec{}
	defer withFakeLauncher(t, ex)()
	res := &fakeResolver{names: []string{"Crunchyroll"}}
	defer withFakeResolver(t, res)()

	var buf bytes.Buffer
	code := runOpen(context.Background(), []string{"Some Anime"}, &buf)
	if code != 0 {
		t.Fatalf("exit=%d, want 0; out=%q", code, buf.String())
	}
	if ex.called != 0 {
		t.Fatalf("launcher should not exec when no mapped provider; called=%d", ex.called)
	}
	out := buf.String()
	if !strings.Contains(out, "Crunchyroll") {
		t.Fatalf("output missing provider name: %q", out)
	}
	if !strings.Contains(strings.ToLower(out), "leah open") {
		t.Fatalf("output missing hint mentioning leah open: %q", out)
	}
}

// Empty TMDB result set → non-zero exit + stderr message.
func TestOpen_TitleNotFound_ExitsOne(t *testing.T) {
	ex := &fakeOpenExec{}
	defer withFakeLauncher(t, ex)()
	res := &fakeResolver{names: nil}
	defer withFakeResolver(t, res)()

	r, w, _ := os.Pipe()
	oldStderr := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = oldStderr }()

	var buf bytes.Buffer
	code := runOpen(context.Background(), []string{"Nonexistent Movie"}, &buf)
	_ = w.Close()
	errBytes := make([]byte, 4096)
	n, _ := r.Read(errBytes)
	if code != 1 {
		t.Fatalf("exit=%d, want 1", code)
	}
	if !strings.Contains(string(errBytes[:n]), "no results") {
		t.Fatalf("stderr missing 'no results': %q", string(errBytes[:n]))
	}
}

// No TMDB token → known intent still works, unknown arg returns the launcher's
// unknown-target error (NOT a TMDB-connect hint — TMDB is optional fallback).
func TestOpen_TMDBTokenMissing_FallsBackTo_LauncherOnly(t *testing.T) {
	ex := &fakeOpenExec{}
	defer withFakeLauncher(t, ex)()
	defer withNoResolver(t, errNoTMDBToken)()

	// Known intent: still works.
	var buf bytes.Buffer
	code := runOpen(context.Background(), []string{"netflix"}, &buf)
	if code != 0 {
		t.Fatalf("known intent without TMDB: exit=%d, want 0", code)
	}
	if ex.called != 1 {
		t.Fatalf("exec called %d, want 1", ex.called)
	}

	// Unknown arg: exit 2 with launcher's unknown-target error.
	r, w, _ := os.Pipe()
	oldStderr := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = oldStderr }()

	buf.Reset()
	code = runOpen(context.Background(), []string{"Game of Thrones"}, &buf)
	_ = w.Close()
	errBytes := make([]byte, 4096)
	n, _ := r.Read(errBytes)
	if code != 2 {
		t.Fatalf("unknown arg without TMDB: exit=%d, want 2", code)
	}
	msg := string(errBytes[:n])
	if !strings.Contains(msg, "unknown target") {
		t.Fatalf("stderr missing 'unknown target': %q", msg)
	}
}

// Resolver error other than missing-token surfaces as exit 1.
func TestOpen_ResolverError_ExitsOne(t *testing.T) {
	ex := &fakeOpenExec{}
	defer withFakeLauncher(t, ex)()
	res := &fakeResolver{err: errors.New("boom")}
	defer withFakeResolver(t, res)()

	r, w, _ := os.Pipe()
	oldStderr := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = oldStderr }()

	var buf bytes.Buffer
	code := runOpen(context.Background(), []string{"Some Title"}, &buf)
	_ = w.Close()
	errBytes := make([]byte, 4096)
	n, _ := r.Read(errBytes)
	if code != 1 {
		t.Fatalf("exit=%d, want 1; stderr=%q", code, string(errBytes[:n]))
	}
}
