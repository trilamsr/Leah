package knowledge

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func newCitationGraph(t *testing.T) *Graph {
	t.Helper()
	clock := time.Date(2026, 6, 22, 0, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	g, err := New(Config{Path: filepath.Join(dir, "k.db"), Now: func() time.Time { return clock }})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = g.Close() })
	return g
}

func TestEnrichCitation_KnownPaperURL(t *testing.T) {
	g := newCitationGraph(t)
	if err := g.Add(Entity{
		Kind:    KindProject,
		Key:     "arxiv:2406.12345",
		Display: "Attention Tweaks",
		Aliases: []string{"https://arxiv.org/abs/2406.12345"},
	}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	enr, err := EnrichCitation(context.Background(), g, "https://arxiv.org/abs/2406.12345")
	if err != nil {
		t.Fatalf("EnrichCitation: %v", err)
	}
	if enr == nil {
		t.Fatal("want enrichment, got nil")
	}
	if enr.EntityKind != KindProject || enr.EntityKey != "arxiv:2406.12345" {
		t.Errorf("entity: %+v", enr)
	}
	if enr.Display != "Attention Tweaks" {
		t.Errorf("display: %q", enr.Display)
	}
	if enr.ArxivID != "2406.12345" {
		t.Errorf("arxiv_id: %q", enr.ArxivID)
	}
	if enr.Domain != "arxiv.org" {
		t.Errorf("domain: %q", enr.Domain)
	}
}

func TestEnrichCitation_UnknownURL_NilNoError(t *testing.T) {
	g := newCitationGraph(t)
	enr, err := EnrichCitation(context.Background(), g, "https://unknown.example.com/x")
	if err != nil {
		t.Fatalf("want nil error, got %v", err)
	}
	if enr != nil {
		t.Fatalf("want nil enrichment, got %+v", enr)
	}
}

func TestEnrichCitation_EmptyURL_NilNoError(t *testing.T) {
	g := newCitationGraph(t)
	enr, err := EnrichCitation(context.Background(), g, "")
	if err != nil {
		t.Fatalf("want nil error, got %v", err)
	}
	if enr != nil {
		t.Fatalf("want nil, got %+v", enr)
	}
}

func TestEnrichCitation_CtxCancel(t *testing.T) {
	g := newCitationGraph(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := EnrichCitation(ctx, g, "https://arxiv.org/abs/2406.12345")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}

func TestEnrichCitation_LookalikeHostRejected(t *testing.T) {
	g := newCitationGraph(t)
	enr, err := EnrichCitation(context.Background(), g, "https://evil-arxiv.org/abs/2406.12345")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if enr != nil {
		t.Fatalf("want nil, got %+v", enr)
	}
}

func TestEnrichCitation_ArxivIDFromBareURL(t *testing.T) {
	g := newCitationGraph(t)
	// No entity persisted — bare arxiv URL still produces a non-nil
	// enrichment carrying the parsed arxiv_id + domain, even when KG
	// has no record. Lets the UI render a useful tile before the KG
	// catches up.
	enr, err := EnrichCitation(context.Background(), g, "https://arxiv.org/abs/2406.12345v2")
	if err != nil {
		t.Fatalf("EnrichCitation: %v", err)
	}
	if enr == nil || enr.ArxivID != "2406.12345" {
		t.Fatalf("arxiv_id: %+v", enr)
	}
	if enr.EntityKey != "" {
		t.Errorf("entity_key should be empty when not in KG: %q", enr.EntityKey)
	}
}
