package main

import (
	"context"
	"os"

	"github.com/trilam/leah/internal/adapters/confluence"
	"github.com/trilam/leah/internal/adapters/jira"
	"github.com/trilam/leah/internal/adapters/linear"
	"github.com/trilam/leah/internal/adapters/notion"
	"github.com/trilam/leah/internal/brief"
	"github.com/trilam/leah/internal/connect"
)

// wireWorkTools adds a brief section per connected work tool. Each lister is
// built only when its paste-token file is present (and, for jira/confluence,
// the host-specific base URL / space is configured) so a never-connected or
// under-configured tool stays silent — absence, not "(unavailable)". Only
// tools with a brief-suitable personal read RPC are wired: jira/linear
// ListMyIssues, notion ListDatabases, confluence ListRecentPages. Slack and
// MS Teams expose no such read (channel inventory / write-only), so omitted.
func wireWorkTools(o *brief.GatherOpts) {
	if connected(connect.DefaultTokenPath("jira")) {
		if l := newJiraLister(); l != nil {
			o.Jira = l
		}
	}
	if connected(connect.DefaultTokenPath("linear")) {
		if l := newLinearLister(); l != nil {
			o.Linear = l
		}
	}
	if connected(connect.DefaultTokenPath("notion")) {
		if l := newNotionLister(); l != nil {
			o.Notion = l
		}
	}
	if connected(connect.DefaultTokenPath("confluence")) {
		if l := newConfluenceLister(); l != nil {
			o.Confluence = l
		}
	}
}

func newJiraLister() brief.WorkLister {
	base := os.Getenv("LEAH_JIRA_BASE_URL")
	if base == "" {
		return nil
	}
	ts, err := connect.LoadStaticSource(connect.DefaultTokenPath("jira"))
	if err != nil {
		return nil
	}
	c, err := jira.New(jira.Config{
		Attestor:    noopAttestor{},
		TokenSource: ts,
		Transport:   jira.NewHTTPTransport(nil, base),
		BaseURL:     base,
	})
	if err != nil {
		return nil
	}
	return jiraLister{c}
}

type jiraLister struct{ c *jira.Client }

func (j jiraLister) List(ctx context.Context) ([]brief.WorkItem, error) {
	issues, err := j.c.ListMyIssues(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]brief.WorkItem, 0, len(issues))
	for _, i := range issues {
		out = append(out, brief.WorkItem{Title: i.Key + " " + i.Summary, Detail: i.Status})
	}
	return out, nil
}

func newLinearLister() brief.WorkLister {
	ts, err := connect.LoadStaticSource(connect.DefaultTokenPath("linear"))
	if err != nil {
		return nil
	}
	c, err := linear.New(linear.Config{
		Attestor:    noopAttestor{},
		TokenSource: ts,
		Transport:   linear.NewHTTPTransport(nil, linear.Endpoint),
	})
	if err != nil {
		return nil
	}
	return linearLister{c}
}

type linearLister struct{ c *linear.Client }

func (l linearLister) List(ctx context.Context) ([]brief.WorkItem, error) {
	issues, err := l.c.ListMyIssues(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]brief.WorkItem, 0, len(issues))
	for _, i := range issues {
		out = append(out, brief.WorkItem{Title: i.Identifier + " " + i.Title, Detail: i.State})
	}
	return out, nil
}

func newNotionLister() brief.WorkLister {
	ts, err := connect.LoadStaticSource(connect.DefaultTokenPath("notion"))
	if err != nil {
		return nil
	}
	a, err := notion.New(notion.Config{
		Attestor:    noopAttestor{},
		TokenSource: ts,
		Transport:   notion.NewHTTPTransport(nil, ""),
	})
	if err != nil {
		return nil
	}
	return notionLister{a}
}

type notionLister struct{ a *notion.Adapter }

func (n notionLister) List(ctx context.Context) ([]brief.WorkItem, error) {
	dbs, err := n.a.ListDatabases(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]brief.WorkItem, 0, len(dbs))
	for _, d := range dbs {
		out = append(out, brief.WorkItem{Title: d.Title})
	}
	return out, nil
}

func newConfluenceLister() brief.WorkLister {
	base, space := os.Getenv("LEAH_CONFLUENCE_BASE_URL"), os.Getenv("LEAH_CONFLUENCE_SPACE")
	if base == "" || space == "" {
		return nil
	}
	ts, err := connect.LoadStaticSource(connect.DefaultTokenPath("confluence"))
	if err != nil {
		return nil
	}
	c, err := confluence.New(confluence.Config{
		Attestor:    noopAttestor{},
		TokenSource: ts,
		Transport:   confluence.NewHTTPTransport(nil, base),
		BaseURL:     base,
	})
	if err != nil {
		return nil
	}
	return confluenceLister{c: c, space: space}
}

type confluenceLister struct {
	c     *confluence.Client
	space string
}

func (cl confluenceLister) List(ctx context.Context) ([]brief.WorkItem, error) {
	pages, err := cl.c.ListRecentPages(ctx, cl.space)
	if err != nil {
		return nil, err
	}
	out := make([]brief.WorkItem, 0, len(pages))
	for _, p := range pages {
		out = append(out, brief.WorkItem{Title: p.Title, Detail: p.SpaceKey})
	}
	return out, nil
}
