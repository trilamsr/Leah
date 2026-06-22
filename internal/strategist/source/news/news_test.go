package news

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/trilam/leah/internal/feeds"
)

type fakeFetcher struct {
	arts []feeds.Article
	err  error
}

func (f *fakeFetcher) Fetch(_ context.Context) ([]feeds.Article, error) {
	return f.arts, f.err
}

func TestNewsSource_FetchesBundle_HappyPath(t *testing.T) {
	now := time.Now()
	ff := &fakeFetcher{arts: []feeds.Article{
		{Title: "AI model X ships", URL: "https://a/1", Source: "hn", Published: now.Add(-time.Minute)},
		{Title: "kernel bug Y", URL: "https://a/2", Source: "lwn", Published: now.Add(-2 * time.Minute)},
		{Title: "earnings Z", URL: "https://a/3", Source: "wsj", Published: now.Add(-3 * time.Minute)},
	}}
	s := New(Config{Fetcher: ff})
	facts, err := s.Candidates(context.Background(), now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("Candidates: %v", err)
	}
	if len(facts) != 3 {
		t.Fatalf("facts = %d, want 3", len(facts))
	}
	if facts[0].Slot != "news" || facts[0].Origin != "https://a/1" {
		t.Errorf("fact[0] = %+v", facts[0])
	}
}

func TestNewsSource_FetchError_WrappedAsSourceError(t *testing.T) {
	ff := &fakeFetcher{err: errors.New("rss timeout")}
	s := New(Config{Fetcher: ff})
	_, err := s.Candidates(context.Background(), time.Now())
	if err == nil {
		t.Fatal("want error")
	}
}

func TestNewsSource_FiltersBySince(t *testing.T) {
	now := time.Now()
	ff := &fakeFetcher{arts: []feeds.Article{
		{Title: "fresh", URL: "u1", Published: now.Add(-time.Minute)},
		{Title: "old", URL: "u2", Published: now.Add(-24 * time.Hour)},
	}}
	s := New(Config{Fetcher: ff})
	facts, err := s.Candidates(context.Background(), now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(facts) != 1 || facts[0].Origin != "u1" {
		t.Fatalf("want only fresh, got %+v", facts)
	}
}

func TestNewsSource_Name(t *testing.T) {
	s := New(Config{Fetcher: &fakeFetcher{}})
	if s.Name() != "news" {
		t.Errorf("Name = %q, want news", s.Name())
	}
}
