package meta

import (
	"strings"
	"testing"
)

func TestParseMeta_OpenGraph(t *testing.T) {
	html := `<html><head>
<meta property="og:title" content="Attention Is All You Need">
<meta property="og:site_name" content="arXiv">
<meta property="og:description" content="Transformer paper">
<meta name="author" content="Vaswani et al.">
<link rel="icon" href="/favicon.ico">
</head></html>`
	m, err := ParseMeta(strings.NewReader(html), "https://arxiv.org/abs/1706.03762")
	if err != nil {
		t.Fatalf("ParseMeta: %v", err)
	}
	if m.Title != "Attention Is All You Need" {
		t.Errorf("Title = %q, want %q", m.Title, "Attention Is All You Need")
	}
	if m.SiteName != "arXiv" {
		t.Errorf("SiteName = %q", m.SiteName)
	}
	if m.Description != "Transformer paper" {
		t.Errorf("Description = %q", m.Description)
	}
	if m.Author != "Vaswani et al." {
		t.Errorf("Author = %q", m.Author)
	}
	if m.Favicon != "https://arxiv.org/favicon.ico" {
		t.Errorf("Favicon = %q", m.Favicon)
	}
}

func TestParseMeta_TitleFallback(t *testing.T) {
	html := `<html><head><title>Plain Title</title></head></html>`
	m, err := ParseMeta(strings.NewReader(html), "https://example.com")
	if err != nil {
		t.Fatalf("ParseMeta: %v", err)
	}
	if m.Title != "Plain Title" {
		t.Errorf("Title fallback = %q", m.Title)
	}
}

func TestParseMeta_EmptyHTML(t *testing.T) {
	m, err := ParseMeta(strings.NewReader(""), "https://example.com")
	if err != nil {
		t.Fatalf("ParseMeta: %v", err)
	}
	if m.Title != "" || m.Author != "" {
		t.Errorf("empty html should give empty meta, got %+v", m)
	}
}
