package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const paperAtomBody = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <entry>
    <id>http://arxiv.org/abs/2406.12345v1</id>
    <title>Test Paper Title</title>
    <summary>Test paper abstract content.</summary>
    <published>2024-06-18T07:22:32Z</published>
    <link href="https://arxiv.org/abs/2406.12345v1" rel="alternate" type="text/html"/>
    <author><name>Alice</name></author>
  </entry>
</feed>`

func TestRunPaper_SaveListRead(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(paperAtomBody))
	}))
	defer srv.Close()

	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	prev := arxivEndpointOverride
	arxivEndpointOverride = srv.URL
	t.Cleanup(func() { arxivEndpointOverride = prev })

	var buf bytes.Buffer
	if code := runPaper(context.Background(), []string{"save", "2406.12345"}, &buf, &buf); code != 0 {
		t.Fatalf("save exit %d, want 0; output=%q", code, buf.String())
	}
	if !strings.Contains(buf.String(), "saved 2406.12345") {
		t.Errorf("save output missing confirmation: %q", buf.String())
	}

	buf.Reset()
	if code := runPaper(context.Background(), []string{"list"}, &buf, &buf); code != 0 {
		t.Fatalf("list exit %d, want 0", code)
	}
	if !strings.Contains(buf.String(), "Test Paper Title") {
		t.Errorf("list missing title: %q", buf.String())
	}
	if !strings.Contains(buf.String(), "unread") {
		t.Errorf("list missing status: %q", buf.String())
	}

	buf.Reset()
	if code := runPaper(context.Background(), []string{"read", "2406.12345"}, &buf, &buf); code != 0 {
		t.Fatalf("read exit %d, want 0; out=%q", code, buf.String())
	}

	buf.Reset()
	if code := runPaper(context.Background(), []string{"list", "--status", "unread"}, &buf, &buf); code != 0 {
		t.Fatalf("list filter exit %d", code)
	}
	if strings.Contains(buf.String(), "Test Paper Title") {
		t.Errorf("after marking read, --status unread should drop it: %q", buf.String())
	}

	buf.Reset()
	if code := runPaper(context.Background(), []string{"list", "--status", "read"}, &buf, &buf); code != 0 {
		t.Fatalf("list read filter exit %d", code)
	}
	if !strings.Contains(buf.String(), "Test Paper Title") {
		t.Errorf("--status read should surface marked paper: %q", buf.String())
	}
}

func TestRunPaper_SaveAcceptsURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(paperAtomBody))
	}))
	defer srv.Close()

	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	prev := arxivEndpointOverride
	arxivEndpointOverride = srv.URL
	t.Cleanup(func() { arxivEndpointOverride = prev })

	var buf bytes.Buffer
	if code := runPaper(context.Background(), []string{"save", "https://arxiv.org/abs/2406.12345v2"}, &buf, &buf); code != 0 {
		t.Fatalf("save URL exit %d, want 0; out=%q", code, buf.String())
	}
	if !strings.Contains(buf.String(), "2406.12345") {
		t.Errorf("URL form should normalize to bare ID: %q", buf.String())
	}
}

func TestRunPaper_Help(t *testing.T) {
	var buf bytes.Buffer
	if code := runPaper(context.Background(), []string{"--help"}, &buf, &buf); code != 0 {
		t.Fatalf("--help exit %d, want 0", code)
	}
	if !strings.Contains(buf.String(), "usage: leah paper") {
		t.Errorf("help missing usage: %q", buf.String())
	}
}

func TestRunPaper_UnknownSubverb(t *testing.T) {
	var buf bytes.Buffer
	if code := runPaper(context.Background(), []string{"nope"}, &buf, &buf); code != 2 {
		t.Fatalf("unknown subverb exit %d, want 2", code)
	}
}

func TestRunPaper_SaveMissingID(t *testing.T) {
	var buf bytes.Buffer
	if code := runPaper(context.Background(), []string{"save"}, &buf, &buf); code != 2 {
		t.Fatalf("save without id exit %d, want 2", code)
	}
}

func TestRunPaper_ListInvalidStatusErrors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	var buf bytes.Buffer
	if code := runPaper(context.Background(), []string{"list", "--status", "garbage"}, &buf, &buf); code != 2 {
		t.Fatalf("invalid --status exit %d, want 2; out=%q", code, buf.String())
	}
	if !strings.Contains(buf.String(), "invalid --status") {
		t.Errorf("expected error message about invalid --status, got %q", buf.String())
	}
}

func TestRunPaper_ReadUnknownID(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	var buf bytes.Buffer
	if code := runPaper(context.Background(), []string{"read", "9999.99999"}, &buf, &buf); code != 1 {
		t.Fatalf("read unknown id exit %d, want 1", code)
	}
}

func TestRunPaper_SaveDedupes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(paperAtomBody))
	}))
	defer srv.Close()

	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	prev := arxivEndpointOverride
	arxivEndpointOverride = srv.URL
	t.Cleanup(func() { arxivEndpointOverride = prev })

	var buf bytes.Buffer
	_ = runPaper(context.Background(), []string{"save", "2406.12345"}, &buf, &buf)
	_ = runPaper(context.Background(), []string{"save", "2406.12345v1"}, &buf, &buf)

	buf.Reset()
	if code := runPaper(context.Background(), []string{"list"}, &buf, &buf); code != 0 {
		t.Fatalf("list exit %d", code)
	}
	if strings.Count(buf.String(), "Test Paper Title") != 1 {
		t.Errorf("dedup failed; list output: %q", buf.String())
	}
}
