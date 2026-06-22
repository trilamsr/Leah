package main

import (
	"strings"
	"testing"
	"time"
)

// TestParseSince_Forms covers the operator-visible `--since` surface across recall/brief/news.
func TestParseSince_Forms(t *testing.T) {
	ts := "2026-06-20T00:00:00Z"
	want, _ := time.Parse(time.RFC3339, ts)
	cases := []struct {
		name    string
		args    []string
		want    time.Time
		rest    []string
		wantErr bool
	}{
		{"equals form", []string{"--since=" + ts, "ship"}, want, []string{"ship"}, false},
		{"space form", []string{"--since", ts, "ship"}, want, []string{"ship"}, false},
		{"absent", []string{"ship"}, time.Time{}, []string{"ship"}, false},
		{"mixed with flag", []string{"--llm", "--since=" + ts, "ship"}, want, []string{"--llm", "ship"}, false},
		{"bare trailing rejects", []string{"ship", "--since"}, time.Time{}, nil, true},
		{"bare only rejects", []string{"--since"}, time.Time{}, nil, true},
		{"empty equals rejects", []string{"--since="}, time.Time{}, nil, true},
		{"repeat rejects", []string{"--since=" + ts, "--since=" + ts}, time.Time{}, nil, true},
		{"invalid rfc3339 rejects", []string{"--since=not-a-date"}, time.Time{}, nil, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, gotRest, err := ParseSince(c.args)
			if (err != nil) != c.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, c.wantErr)
			}
			if c.wantErr {
				return
			}
			if !got.Equal(c.want) {
				t.Errorf("since: got %v want %v", got, c.want)
			}
			if strings.Join(gotRest, " ") != strings.Join(c.rest, " ") {
				t.Errorf("rest: got %v want %v", gotRest, c.rest)
			}
		})
	}
}
