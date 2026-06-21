package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// graphqlPayload builds the gh api search response shape so tests don't repeat
// the nested {data.search.nodes:[...]} envelope.
func graphqlPayload(nodes []map[string]any) string {
	body := map[string]any{
		"data": map[string]any{
			"search": map[string]any{
				"nodes": nodes,
			},
		},
	}
	raw, _ := json.Marshal(body)
	return string(raw)
}

// TestRankReviewQueue pins the rank invariant: oldest review-request first
// (FIFO inbox), drafts sink below ready PRs regardless of age.
func TestRankReviewQueue(t *testing.T) {
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	rows := []reviewRow{
		{Number: 1, UpdatedAt: now.Add(-2 * time.Hour), IsDraft: false},
		{Number: 2, UpdatedAt: now.Add(-24 * time.Hour), IsDraft: false},
		{Number: 3, UpdatedAt: now.Add(-48 * time.Hour), IsDraft: true},
		{Number: 4, UpdatedAt: now.Add(-6 * time.Hour), IsDraft: false},
	}
	rankReviewQueue(rows)
	if rows[0].Number != 2 {
		t.Errorf("rank[0] = #%d, want #2 (oldest non-draft)", rows[0].Number)
	}
	if rows[len(rows)-1].Number != 3 {
		t.Errorf("rank[last] = #%d, want #3 (draft sinks)", rows[len(rows)-1].Number)
	}
}

// TestFormatReviewRow pins the one-line shape. Width matters less than
// visible signals: repo/number/title/age/draft-flag.
func TestFormatReviewRow(t *testing.T) {
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	r := reviewRow{
		Number:    289,
		Repo:      "trilamsr/Leah",
		Title:     "feat(cli): leah news --bundle",
		UpdatedAt: now.Add(-3 * time.Hour),
	}
	got := formatReviewRow(r, now)
	for _, want := range []string{"#289", "trilamsr/Leah", "feat(cli)", "3h"} {
		if !strings.Contains(got, want) {
			t.Errorf("formatReviewRow missing %q in %q", want, got)
		}
	}
}

func TestFormatReviewRow_DraftFlag(t *testing.T) {
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	r := reviewRow{Number: 7, Repo: "x/y", Title: "wip", UpdatedAt: now.Add(-1 * time.Hour), IsDraft: true}
	got := formatReviewRow(r, now)
	if !strings.Contains(got, "DRAFT") {
		t.Errorf("draft PR should be flagged; got %q", got)
	}
}

// TestFormatReviewRow_TitleTruncated guards the terminal one-line constraint —
// long titles must not wrap and trash the operator's eye-scan.
func TestFormatReviewRow_TitleTruncated(t *testing.T) {
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	long := strings.Repeat("a", 200)
	r := reviewRow{Number: 1, Repo: "x/y", Title: long, UpdatedAt: now}
	got := formatReviewRow(r, now)
	if strings.Contains(got, long) {
		t.Errorf("title not truncated; line len=%d", len(got))
	}
	if !strings.Contains(got, "…") {
		t.Errorf("truncated title should carry ellipsis; got %q", got)
	}
}

func TestFetchReviewQueue(t *testing.T) {
	nodes := []map[string]any{
		{
			"number":     289,
			"title":      "feat: x",
			"url":        "https://github.com/trilamsr/Leah/pull/289",
			"updatedAt":  "2026-06-21T09:00:00Z",
			"isDraft":    false,
			"repository": map[string]any{"nameWithOwner": "trilamsr/Leah"},
		},
		{
			"number":     5,
			"title":      "wip",
			"url":        "https://github.com/foo/bar/pull/5",
			"updatedAt":  "2026-06-20T09:00:00Z",
			"isDraft":    true,
			"repository": map[string]any{"nameWithOwner": "foo/bar"},
		},
	}
	fx := &fakeExec{responses: map[string]string{
		"gh api graphql": graphqlPayload(nodes),
	}}
	got, err := fetchReviewQueue(context.Background(), fx, "")
	if err != nil {
		t.Fatalf("fetchReviewQueue: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Number != 289 {
		t.Errorf("got[0].Number = %d, want 289", got[0].Number)
	}
	if got[0].Repo != "trilamsr/Leah" {
		t.Errorf("got[0].Repo = %q, want trilamsr/Leah", got[0].Repo)
	}
	if !got[1].IsDraft {
		t.Errorf("got[1] should be draft")
	}
}

// TestFetchReviewQueue_OrgFilter checks the --org scoping reaches the gh
// query string. Server-side filter is cheaper than client-side filter for a
// search that already supports `org:` qualifier.
func TestFetchReviewQueue_OrgFilter(t *testing.T) {
	var captured []string
	fx := &captureExec{out: graphqlPayload(nil), capture: &captured}
	_, err := fetchReviewQueue(context.Background(), fx, "acme")
	if err != nil {
		t.Fatalf("fetchReviewQueue: %v", err)
	}
	joined := strings.Join(captured, " ")
	if !strings.Contains(joined, "org:acme") {
		t.Errorf("--org acme should add org:acme to query; got %q", joined)
	}
}

// TestRunReviewQueueEmpty pins the silent-zero exit shape so the verb is
// pipeline-safe (no spurious "no PRs" line breaking grep -q).
func TestRunReviewQueueEmpty(t *testing.T) {
	fx := &fakeExec{responses: map[string]string{"gh api graphql": graphqlPayload(nil)}}
	var buf strings.Builder
	rc := runReviewQueueWith(context.Background(), fx, []string{}, &buf)
	if rc != 0 {
		t.Errorf("empty queue exit = %d, want 0", rc)
	}
	if buf.Len() != 0 {
		t.Errorf("empty queue should be silent; got %q", buf.String())
	}
}

func TestRunReviewQueueJSON(t *testing.T) {
	nodes := []map[string]any{
		{
			"number":     1,
			"title":      "x",
			"url":        "https://github.com/r/r/pull/1",
			"updatedAt":  "2026-06-20T09:00:00Z",
			"isDraft":    false,
			"repository": map[string]any{"nameWithOwner": "r/r"},
		},
	}
	fx := &fakeExec{responses: map[string]string{"gh api graphql": graphqlPayload(nodes)}}
	var buf strings.Builder
	rc := runReviewQueueWith(context.Background(), fx, []string{"--json"}, &buf)
	if rc != 0 {
		t.Errorf("--json exit = %d, want 0", rc)
	}
	var arr []map[string]any
	if err := json.Unmarshal([]byte(buf.String()), &arr); err != nil {
		t.Fatalf("invalid json output: %v\n%s", err, buf.String())
	}
	if len(arr) != 1 || arr[0]["number"].(float64) != 1 {
		t.Errorf("json shape wrong: %v", arr)
	}
}

func TestRunReviewQueueHelp(t *testing.T) {
	var buf strings.Builder
	rc := runReviewQueue(context.Background(), []string{"--help"}, &buf)
	if rc != 0 {
		t.Errorf("--help exit = %d, want 0", rc)
	}
	if !strings.Contains(buf.String(), "usage:") {
		t.Errorf("--help output missing usage line: %q", buf.String())
	}
}

func TestRunReviewQueueBadFlag(t *testing.T) {
	var buf strings.Builder
	rc := runReviewQueueWith(context.Background(), &fakeExec{}, []string{"--nope"}, &buf)
	if rc != 2 {
		t.Errorf("bad flag exit = %d, want 2", rc)
	}
}

// captureExec records the gh args it was called with so we can assert on
// query content (e.g. org: qualifier) without depending on response shape.
type captureExec struct {
	out     string
	capture *[]string
}

func (c *captureExec) Run(_ context.Context, args []string) (string, error) {
	*c.capture = append(*c.capture, args...)
	return c.out, nil
}
