package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/trilam/leah/internal/strategist/store"
)

// fakeClipboard captures pbcopy stdin in-process so tests never shell out and
// adversarial inputs (shell metacharacters in topics) are observed as raw
// bytes rather than interpreted by a shell.
type fakeClipboard struct{ got string }

func (f *fakeClipboard) Copy(_ context.Context, body string) error {
	f.got = body
	return nil
}

// fakeLookPath maps tool names to "found"/"not found" without touching $PATH —
// the doctor tests run on machines whose ffmpeg/magick presence is unknown.
type fakeLookPath map[string]bool

func (f fakeLookPath) Look(name string) (string, error) {
	if f[name] {
		return "/usr/local/bin/" + name, nil
	}
	return "", os.ErrNotExist
}

// writeHiggsfieldToken stages the connect-shaped JSON under stateDir so doctor's
// token-presence probe finds it via connect.DefaultTokenPath.
func writeHiggsfieldToken(t *testing.T, dir string) {
	t.Helper()
	path := filepath.Join(dir, "secrets", "higgsfield-token.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"access_token":"hf_test"}`), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}
}

// writeOpenAIToken stages the openai token sentinel doctor looks for. v1 of
// the strategist doesn't actually CALL openai (voiceover deferred to v2) but
// doctor still surfaces it as a forward-looking readiness signal.
func writeOpenAIToken(t *testing.T, dir string) {
	t.Helper()
	path := filepath.Join(dir, "secrets", "openai-token.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"access_token":"sk-test"}`), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}
}

// writePersona stages persona.md under $LEAH_CONFIG_DIR/strategist/persona.md.
func writePersona(t *testing.T, configDir, body string) {
	t.Helper()
	path := filepath.Join(configDir, "strategist", "persona.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write persona: %v", err)
	}
}

// TestStrategistDoctor_HappyPath_PrintsOKLines — all five probes green, exit 0.
func TestStrategistDoctor_HappyPath_PrintsOKLines(t *testing.T) {
	stateDir := t.TempDir()
	configDir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", stateDir)
	t.Setenv("LEAH_CONFIG_DIR", configDir)
	writePersona(t, configDir, "test voice")
	writeHiggsfieldToken(t, stateDir)
	writeOpenAIToken(t, stateDir)

	deps := strategistDeps{
		look: fakeLookPath{"ffmpeg": true, "magick": true},
	}
	var buf bytes.Buffer
	code := runStrategistDoctor(deps, &buf)
	if code != 0 {
		t.Fatalf("exit %d, want 0; out=%q", code, buf.String())
	}
	for _, want := range []string{"persona", "ffmpeg", "magick", "higgsfield", "openai", "OK"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("missing %q in doctor output:\n%s", want, buf.String())
		}
	}
}

// TestStrategistDoctor_MissingPersona_PrintsWarnExit1 — persona file absent.
func TestStrategistDoctor_MissingPersona_PrintsWarnExit1(t *testing.T) {
	stateDir := t.TempDir()
	configDir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", stateDir)
	t.Setenv("LEAH_CONFIG_DIR", configDir)
	writeHiggsfieldToken(t, stateDir)
	writeOpenAIToken(t, stateDir)

	deps := strategistDeps{
		look: fakeLookPath{"ffmpeg": true, "magick": true},
	}
	var buf bytes.Buffer
	code := runStrategistDoctor(deps, &buf)
	if code == 0 {
		t.Fatalf("expected nonzero exit, got 0; out=%q", buf.String())
	}
	if !strings.Contains(buf.String(), "persona") {
		t.Errorf("missing persona line: %q", buf.String())
	}
}

// TestStrategistDoctor_MissingTools_PrintsExit1 — ffmpeg/magick absent.
func TestStrategistDoctor_MissingTools_PrintsExit1(t *testing.T) {
	stateDir := t.TempDir()
	configDir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", stateDir)
	t.Setenv("LEAH_CONFIG_DIR", configDir)
	writePersona(t, configDir, "voice")
	writeHiggsfieldToken(t, stateDir)
	writeOpenAIToken(t, stateDir)

	deps := strategistDeps{
		look: fakeLookPath{}, // neither ffmpeg nor magick
	}
	var buf bytes.Buffer
	code := runStrategistDoctor(deps, &buf)
	if code == 0 {
		t.Fatalf("expected nonzero exit, got 0; out=%q", buf.String())
	}
	out := buf.String()
	if !strings.Contains(out, "ffmpeg") || !strings.Contains(out, "magick") {
		t.Errorf("expected ffmpeg+magick lines: %q", out)
	}
}

// TestStrategistPost_WritesToStoreAndCopiesToClipboard — MVP stub: placeholder
// body lands in inbox/<id>.md, clipboard receives the body. Topic with shell
// metacharacters MUST be copied verbatim — the clipboard path must not shell-escape.
func TestStrategistPost_WritesToStoreAndCopiesToClipboard(t *testing.T) {
	stateDir := t.TempDir()
	configDir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", stateDir)
	t.Setenv("LEAH_CONFIG_DIR", configDir)
	writePersona(t, configDir, "voice")

	clip := &fakeClipboard{}
	deps := strategistDeps{clip: clip, now: func() time.Time { return time.Unix(1700000000, 0) }}

	topic := "shipping leah strategist `whoami` $HOME"
	var buf bytes.Buffer
	code := runStrategistPost(deps, []string{topic}, &buf)
	if code != 0 {
		t.Fatalf("exit %d, want 0; out=%q", code, buf.String())
	}
	if !strings.Contains(clip.got, topic) {
		t.Errorf("clipboard missing verbatim topic: got %q, want substring %q", clip.got, topic)
	}
	// One item under inbox/ (MVP stub uses Inbox not Enqueue — operator reviews
	// before promoting to queue/ via the inbox-approve subcommand).
	m, err := store.Open(filepath.Join(configDir, "strategist"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	items, err := m.ListInbox()
	if err != nil {
		t.Fatalf("list inbox: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("inbox = %d items, want 1", len(items))
	}
	if !strings.Contains(items[0].Text, topic) {
		t.Errorf("inbox body missing topic: %q", items[0].Text)
	}
}

// TestStrategistInbox_ListsInboxItems_Sorted — older first by filename (ULIDs sort).
func TestStrategistInbox_ListsInboxItems_Sorted(t *testing.T) {
	stateDir := t.TempDir()
	configDir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", stateDir)
	t.Setenv("LEAH_CONFIG_DIR", configDir)

	m, err := store.Open(filepath.Join(configDir, "strategist"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	for _, id := range []string{"01J8A0000000000000000000AA", "01J8A0000000000000000000BB", "01J8A0000000000000000000CC"} {
		if err := m.Inbox(store.Item{Schema: store.SchemaCurrent, ID: id, Channel: "linkedin", Slot: "commit", Text: "body " + id, Created: time.Now()}); err != nil {
			t.Fatalf("inbox %s: %v", id, err)
		}
	}

	deps := strategistDeps{}
	var buf bytes.Buffer
	code := runStrategistInbox(deps, nil, &buf)
	if code != 0 {
		t.Fatalf("exit %d, want 0", code)
	}
	out := buf.String()
	aIdx := strings.Index(out, "AA")
	bIdx := strings.Index(out, "BB")
	cIdx := strings.Index(out, "CC")
	if aIdx >= bIdx || bIdx >= cIdx {
		t.Errorf("expected AA < BB < CC ordering, got positions %d %d %d in %q", aIdx, bIdx, cIdx, out)
	}
}

// TestStrategistQueue_PeekN — --peek N caps the listing.
func TestStrategistQueue_PeekN(t *testing.T) {
	stateDir := t.TempDir()
	configDir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", stateDir)
	t.Setenv("LEAH_CONFIG_DIR", configDir)

	m, err := store.Open(filepath.Join(configDir, "strategist"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	for i, id := range []string{
		"01J8B0000000000000000000AA",
		"01J8B0000000000000000000BB",
		"01J8B0000000000000000000CC",
		"01J8B0000000000000000000DD",
	} {
		if err := m.Enqueue(store.Item{Schema: store.SchemaCurrent, ID: id, Channel: "linkedin", Slot: "commit", Text: "body", Origin: "sha" + string(rune('A'+i)), Created: time.Now()}); err != nil {
			t.Fatalf("enqueue: %v", err)
		}
	}

	deps := strategistDeps{}
	var buf bytes.Buffer
	code := runStrategistQueue(deps, []string{"--peek", "2"}, &buf)
	if code != 0 {
		t.Fatalf("exit %d, want 0", code)
	}
	out := buf.String()
	// First two IDs present, last two absent.
	if !strings.Contains(out, "AA") || !strings.Contains(out, "BB") {
		t.Errorf("first 2 IDs missing: %q", out)
	}
	if strings.Contains(out, "CC") || strings.Contains(out, "DD") {
		t.Errorf("--peek 2 should not list IDs beyond N: %q", out)
	}
}

// TestStrategistNext_PopsFromQueueAndMovesToSent — item transitions queue→sent.
func TestStrategistNext_PopsFromQueueAndMovesToSent(t *testing.T) {
	stateDir := t.TempDir()
	configDir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", stateDir)
	t.Setenv("LEAH_CONFIG_DIR", configDir)

	m, err := store.Open(filepath.Join(configDir, "strategist"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	id := "01J8C0000000000000000000ZZ"
	if err := m.Enqueue(store.Item{Schema: store.SchemaCurrent, ID: id, Channel: "linkedin", Slot: "commit", Text: "the body", Created: time.Now()}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	deps := strategistDeps{}
	var buf bytes.Buffer
	code := runStrategistNext(deps, nil, &buf)
	if code != 0 {
		t.Fatalf("exit %d, want 0; out=%q", code, buf.String())
	}
	if !strings.Contains(buf.String(), "the body") {
		t.Errorf("body not printed: %q", buf.String())
	}

	// queue/<id>.md should be gone; sent/<id>.md should exist.
	root := filepath.Join(configDir, "strategist")
	if _, err := os.Stat(filepath.Join(root, "queue", id+".md")); !os.IsNotExist(err) {
		t.Errorf("expected queue/%s.md removed; stat err=%v", id, err)
	}
	if _, err := os.Stat(filepath.Join(root, "sent", id+".md")); err != nil {
		t.Errorf("expected sent/%s.md present; stat err=%v", id, err)
	}
}

// TestStrategistNext_Peek_DoesNotMutate — --peek prints but leaves the queue alone.
func TestStrategistNext_Peek_DoesNotMutate(t *testing.T) {
	stateDir := t.TempDir()
	configDir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", stateDir)
	t.Setenv("LEAH_CONFIG_DIR", configDir)

	m, err := store.Open(filepath.Join(configDir, "strategist"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	id := "01J8D0000000000000000000YY"
	if err := m.Enqueue(store.Item{Schema: store.SchemaCurrent, ID: id, Channel: "linkedin", Slot: "commit", Text: "peeked body", Created: time.Now()}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	deps := strategistDeps{}
	var buf bytes.Buffer
	code := runStrategistNext(deps, []string{"--peek"}, &buf)
	if code != 0 {
		t.Fatalf("exit %d, want 0; out=%q", code, buf.String())
	}
	if !strings.Contains(buf.String(), "peeked body") {
		t.Errorf("body not printed: %q", buf.String())
	}
	root := filepath.Join(configDir, "strategist")
	if _, err := os.Stat(filepath.Join(root, "queue", id+".md")); err != nil {
		t.Errorf("expected queue/%s.md still present after --peek; stat err=%v", id, err)
	}
}

// TestStrategistNext_EmptyQueue_ExitsThree — no queued items → exit 3 per spec.
func TestStrategistNext_EmptyQueue_ExitsThree(t *testing.T) {
	stateDir := t.TempDir()
	configDir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", stateDir)
	t.Setenv("LEAH_CONFIG_DIR", configDir)

	deps := strategistDeps{}
	var buf bytes.Buffer
	code := runStrategistNext(deps, nil, &buf)
	if code != 3 {
		t.Fatalf("exit %d, want 3 for empty queue", code)
	}
}
