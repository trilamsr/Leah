package papers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const atomBody = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <entry>
    <id>http://arxiv.org/abs/2406.12345v1</id>
    <title>Sample Title</title>
    <summary>This is the abstract body.</summary>
    <published>2024-06-18T07:22:32Z</published>
    <link href="https://arxiv.org/abs/2406.12345v1" rel="alternate" type="text/html"/>
    <author><name>Alice Author</name></author>
    <author><name>Bob Builder</name></author>
  </entry>
</feed>`

func TestNormalizeArxivID(t *testing.T) {
	cases := []struct{ in, want string }{
		{"2406.12345", "2406.12345"},
		{"2406.12345v1", "2406.12345"},
		{"2406.12345v23", "2406.12345"},
		{"https://arxiv.org/abs/2406.12345", "2406.12345"},
		{"https://arxiv.org/abs/2406.12345v2", "2406.12345"},
		{"http://arxiv.org/abs/2406.12345v1", "2406.12345"},
		{"https://arxiv.org/pdf/2406.12345v1", "2406.12345"},
		{"https://arxiv.org/pdf/2406.12345v1.pdf", "2406.12345"},
		{" 2406.12345 ", "2406.12345"},
		// Old-style category-prefixed IDs (still valid for pre-2007 papers).
		{"hep-th/9901001", "hep-th/9901001"},
		{"https://arxiv.org/abs/hep-th/9901001v2", "hep-th/9901001"},
	}
	for _, c := range cases {
		got, err := NormalizeArxivID(c.in)
		if err != nil {
			t.Errorf("NormalizeArxivID(%q) err: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("NormalizeArxivID(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNormalizeArxivID_Invalid(t *testing.T) {
	cases := []string{"", "   ", "not-an-id", "https://example.com/", "abs/2406"}
	for _, c := range cases {
		if _, err := NormalizeArxivID(c); err == nil {
			t.Errorf("NormalizeArxivID(%q) expected error, got nil", c)
		}
	}
}

func TestFetchMetadata_Happy(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RequestURI()
		_, _ = w.Write([]byte(atomBody))
	}))
	defer srv.Close()

	f := &ArxivFetcher{Endpoint: srv.URL, Client: srv.Client()}
	p, err := f.FetchMetadata(context.Background(), "2406.12345")
	if err != nil {
		t.Fatalf("FetchMetadata: %v", err)
	}
	if !strings.Contains(gotPath, "id_list=2406.12345") {
		t.Errorf("query missing id_list=2406.12345: %q", gotPath)
	}
	if p.Title != "Sample Title" {
		t.Errorf("title: %q", p.Title)
	}
	if !strings.Contains(p.Abstract, "abstract body") {
		t.Errorf("abstract: %q", p.Abstract)
	}
	if len(p.Authors) != 2 || p.Authors[0] != "Alice Author" || p.Authors[1] != "Bob Builder" {
		t.Errorf("authors: %v", p.Authors)
	}
	if p.Link != "https://arxiv.org/abs/2406.12345v1" {
		t.Errorf("link: %q", p.Link)
	}
	if p.ID != "2406.12345" {
		t.Errorf("ID: %q", p.ID)
	}
}

func TestFetchMetadata_NoEntry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<?xml version="1.0"?><feed xmlns="http://www.w3.org/2005/Atom"></feed>`))
	}))
	defer srv.Close()

	f := &ArxivFetcher{Endpoint: srv.URL, Client: srv.Client()}
	if _, err := f.FetchMetadata(context.Background(), "2406.12345"); err == nil {
		t.Fatalf("expected error on empty feed")
	}
}

func TestFetchMetadata_NonOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	f := &ArxivFetcher{Endpoint: srv.URL, Client: srv.Client()}
	if _, err := f.FetchMetadata(context.Background(), "2406.12345"); err == nil {
		t.Fatalf("expected error on 500")
	}
}
