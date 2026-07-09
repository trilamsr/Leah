package brief

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeWorkLister struct {
	items []WorkItem
	err   error
}

func (f *fakeWorkLister) List(ctx context.Context) ([]WorkItem, error) { return f.items, f.err }

func TestGatherCallsJiraLister(t *testing.T) {
	dir := t.TempDir()
	j := &fakeWorkLister{items: []WorkItem{{Title: "ENG-1", Detail: "open"}}}
	d := Gather(context.Background(), time.Now(), dir, nil, GatherOpts{Jira: j})
	if len(d.JiraItems) != 1 || d.JiraUnavailable {
		t.Errorf("jira items not propagated: %+v unavail=%v", d.JiraItems, d.JiraUnavailable)
	}
}

func TestGatherJiraErrorMarksUnavailable(t *testing.T) {
	dir := t.TempDir()
	j := &fakeWorkLister{err: errors.New("boom")}
	d := Gather(context.Background(), time.Now(), dir, nil, GatherOpts{Jira: j})
	if !d.JiraUnavailable || len(d.JiraItems) != 0 {
		t.Errorf("expected unavailable flag, got %+v unavail=%v", d.JiraItems, d.JiraUnavailable)
	}
}

func TestGatherCallsConfluenceLister(t *testing.T) {
	dir := t.TempDir()
	c := &fakeWorkLister{items: []WorkItem{{Title: "Runbook"}}}
	d := Gather(context.Background(), time.Now(), dir, nil, GatherOpts{Confluence: c})
	if len(d.ConfluenceItems) != 1 || d.ConfluenceUnavailable {
		t.Errorf("confluence items not propagated: %+v unavail=%v", d.ConfluenceItems, d.ConfluenceUnavailable)
	}
}

func TestGatherNilWorkListers_OmitSections(t *testing.T) {
	d := Data{Now: time.Now()}
	out := Render(d)
	for _, h := range []string{"## Jira", "## Confluence"} {
		if strings.Contains(out, h) {
			t.Errorf("unconfigured work tool should omit %q:\n%s", h, out)
		}
	}
}

func TestRenderJiraItems(t *testing.T) {
	d := Data{Now: time.Now(), JiraItems: []WorkItem{{Title: "ENG-42", Detail: "In Progress"}}}
	out := Render(d)
	if !strings.Contains(out, "## Jira") || !strings.Contains(out, "ENG-42") || !strings.Contains(out, "In Progress") {
		t.Errorf("jira section missing content:\n%s", out)
	}
}

func TestRenderWorkToolUnavailable(t *testing.T) {
	d := Data{Now: time.Now(), ConfluenceUnavailable: true}
	out := Render(d)
	if !strings.Contains(out, "## Confluence") || !strings.Contains(out, "unavailable") {
		t.Errorf("expected confluence unavailable fallback:\n%s", out)
	}
}

func TestGatherWorkToolErrorIsolation(t *testing.T) {
	dir := t.TempDir()
	opts := GatherOpts{
		Jira:       &fakeWorkLister{err: errors.New("boom")},
		Confluence: &fakeWorkLister{items: []WorkItem{{Title: "ok"}}},
	}
	d := Gather(context.Background(), time.Now(), dir, nil, opts)
	if !d.JiraUnavailable {
		t.Errorf("jira error should mark JiraUnavailable")
	}
	if d.ConfluenceUnavailable || len(d.ConfluenceItems) != 1 {
		t.Errorf("confluence sibling poisoned by jira error: unavail=%v items=%v", d.ConfluenceUnavailable, d.ConfluenceItems)
	}
}
