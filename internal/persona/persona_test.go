package persona

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/trilam/leah/internal/ctxmgr"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "memory.db")
	// Bring the ctxmgr schema online so workspace_persona FK targets exist.
	m, err := activectx.Open(path)
	if err != nil {
		t.Fatalf("ctxmgr open: %v", err)
	}
	_ = m.Close()
	s, err := Open(path)
	if err != nil {
		t.Fatalf("persona open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// TestLoadUnknownReturnsDefaults asserts a workspace with no persona row
// gets the package-level defaults rather than an error — callers must
// always receive a usable persona.
func TestLoadUnknownReturnsDefaults(t *testing.T) {
	s := openTestStore(t)
	p, err := s.Load("nope")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if p.Tone != DefaultTone || p.Signature != DefaultSignature || p.VoiceID != DefaultVoiceID {
		t.Errorf("missing-row should return defaults, got %+v", p)
	}
	if !p.IsDefault {
		t.Errorf("IsDefault should mark fallback row")
	}
}

// TestSetThenLoad asserts a round-trip on Set + Load preserves all fields.
func TestSetThenLoad(t *testing.T) {
	s := openTestStore(t)
	want := Persona{
		Workspace: "acme",
		Tone:      "formal, concise, no emoji",
		Signature: "— Tri (Acme)",
		VoiceID:   "11labs:acme-voice-id",
	}
	if err := s.Set(want); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, err := s.Load("acme")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Tone != want.Tone || got.Signature != want.Signature || got.VoiceID != want.VoiceID {
		t.Errorf("roundtrip mismatch: got %+v want %+v", got, want)
	}
	if got.IsDefault {
		t.Errorf("explicit row marked IsDefault: %+v", got)
	}
}

// TestSetIsUpsert asserts Set on an existing workspace updates fields rather
// than erroring on PK collision — operator runs `leah workspace persona set`
// repeatedly to tune.
func TestSetIsUpsert(t *testing.T) {
	s := openTestStore(t)
	if err := s.Set(Persona{Workspace: "acme", Tone: "warm"}); err != nil {
		t.Fatalf("set1: %v", err)
	}
	if err := s.Set(Persona{Workspace: "acme", Tone: "formal", Signature: "— Tri"}); err != nil {
		t.Fatalf("set2: %v", err)
	}
	got, _ := s.Load("acme")
	if got.Tone != "formal" || got.Signature != "— Tri" {
		t.Errorf("upsert lost update: %+v", got)
	}
}

// TestSetRejectsEmptyWorkspace asserts Set guards the FK target.
func TestSetRejectsEmptyWorkspace(t *testing.T) {
	s := openTestStore(t)
	err := s.Set(Persona{Workspace: "", Tone: "x"})
	if !errors.Is(err, ErrEmptyWorkspace) {
		t.Fatalf("want ErrEmptyWorkspace, got %v", err)
	}
}

// TestLoadEmptyWorkspaceReturnsDefaults asserts empty input yields defaults
// (callers may pass active workspace name without checking for empty).
func TestLoadEmptyWorkspaceReturnsDefaults(t *testing.T) {
	s := openTestStore(t)
	p, err := s.Load("")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if p.Tone != DefaultTone {
		t.Errorf("empty workspace should return defaults, got %+v", p)
	}
}

// TestSystemPromptPrefixComposesPersona asserts the helper produces a
// single-line preamble suitable for Reasoner.SystemPrompt prefixing.
func TestSystemPromptPrefixComposesPersona(t *testing.T) {
	p := Persona{Workspace: "acme", Tone: "formal", Signature: "— Tri (Acme)"}
	got := p.SystemPromptPrefix()
	if got == "" {
		t.Fatal("empty prefix")
	}
	if !containsAll(got, "acme", "formal", "— Tri (Acme)") {
		t.Errorf("prefix missing fields: %q", got)
	}
}

// TestSystemPromptPrefixSkipsDefaultPersona asserts a default-fallback
// persona contributes an empty prefix (avoid noisy "tone: neutral" prepended
// to every system prompt for unconfigured workspaces).
func TestSystemPromptPrefixSkipsDefaultPersona(t *testing.T) {
	p := Persona{Workspace: "default", Tone: DefaultTone, Signature: DefaultSignature, VoiceID: DefaultVoiceID, IsDefault: true}
	if got := p.SystemPromptPrefix(); got != "" {
		t.Errorf("default persona should produce empty prefix, got %q", got)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		found := false
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
