package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/trilam/leah/internal/adapters/jira"
	"github.com/trilam/leah/internal/adapters/linear"
	"github.com/trilam/leah/internal/adapters/msteams"
	"github.com/trilam/leah/internal/adapters/notion"
	"github.com/trilam/leah/internal/adapters/slack"
	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/connect"
	"github.com/trilam/leah/internal/dispatcher"
)

var errNoWriteSurface = errors.New("no write surface in MVP")

type shipToolFlag struct {
	tool string
	val  *string
}

// runShipTool attests via the ship-tool scope, then calls the tool's write RPC.
func runShipTool(ctx context.Context, tool, title string) int {
	a := &audit.Logger{Path: filepath.Join(stateDir(), "audit.jsonl"), DefaultWorkspace: activeWorkspace}
	w, err := newToolWriter(tool)
	if err != nil {
		if errors.Is(err, errNoWriteSurface) {
			_, _ = fmt.Fprintf(os.Stderr, "leah ship --%s: %v (read-only adapter)\n", tool, err)
		} else {
			_, _ = fmt.Fprintf(os.Stderr, "leah ship --%s: %v\n", tool, err)
		}
		return 2
	}
	st := &dispatcher.ShipTool{Tool: tool, Attestor: newConnectAttestor(), Writer: w, Audit: a, Out: os.Stdout}
	if err := st.Run(ctx, title); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	return 0
}

// newToolWriter binds one tool flag to its producer-verified write RPC; Confluence has none.
func newToolWriter(tool string) (dispatcher.ToolWriter, error) {
	hc := http.DefaultClient
	ts := staticTokenForName(tool)
	switch tool {
	case "slack":
		ad, err := slack.New(slack.Config{Attestor: noopAttestor{}, TokenSource: slackToken{ts}, Transport: slack.NewHTTPTransport(hc, "https://slack.com/api")})
		if err != nil {
			return nil, err
		}
		ch := os.Getenv("LEAH_SHIP_SLACK_CHANNEL")
		return writerFn(func(ctx context.Context, title string) error { return ad.PostMessage(ctx, ch, title) }), nil
	case "jira":
		base := os.Getenv("LEAH_SHIP_JIRA_BASE_URL")
		ad, err := jira.New(jira.Config{Attestor: noopAttestor{}, TokenSource: ts, Transport: jira.NewHTTPTransport(hc, base), BaseURL: base})
		if err != nil {
			return nil, err
		}
		proj := os.Getenv("LEAH_SHIP_JIRA_PROJECT")
		return writerFn(func(ctx context.Context, title string) error {
			_, err := ad.CreateIssue(ctx, jira.IssueReq{ProjectKey: proj, Summary: title, IssueType: "Task"})
			return err
		}), nil
	case "notion":
		ad, err := notion.New(notion.Config{Attestor: noopAttestor{}, TokenSource: ts, Transport: notion.NewHTTPTransport(hc, "https://api.notion.com")})
		if err != nil {
			return nil, err
		}
		db := os.Getenv("LEAH_SHIP_NOTION_DB")
		return writerFn(func(ctx context.Context, title string) error {
			_, err := ad.CreatePage(ctx, db, notion.Props{Title: title})
			return err
		}), nil
	case "linear":
		ad, err := linear.New(linear.Config{Attestor: noopAttestor{}, TokenSource: ts, Transport: linear.NewHTTPTransport(hc, "https://api.linear.app/graphql")})
		if err != nil {
			return nil, err
		}
		team := os.Getenv("LEAH_SHIP_LINEAR_TEAM")
		return writerFn(func(ctx context.Context, title string) error {
			_, err := ad.CreateIssue(ctx, linear.IssueReq{TeamID: team, Title: title})
			return err
		}), nil
	case "teams":
		ad, err := msteams.New(msteams.Config{Attestor: noopAttestor{}, TokenSource: ts, HTTPClient: hc})
		if err != nil {
			return nil, err
		}
		teamID := os.Getenv("LEAH_SHIP_TEAMS_TEAM")
		chID := os.Getenv("LEAH_SHIP_TEAMS_CHANNEL")
		return writerFn(func(ctx context.Context, title string) error { return ad.PostMessage(ctx, teamID, chID, title) }), nil
	case "confluence":
		return nil, errNoWriteSurface
	default:
		return nil, fmt.Errorf("unknown tool %q", tool)
	}
}

type writerFn func(ctx context.Context, title string) error

func (f writerFn) Write(ctx context.Context, title string) error { return f(ctx, title) }

// noopAttestor satisfies each adapter's per-RPC gate; the CLI ship-tool gate is the consent.
type noopAttestor struct{}

func (noopAttestor) Attest(context.Context, string) error { return nil }

type staticToken string

func (t staticToken) Token(context.Context) (string, error) {
	if t == "" {
		return "", errors.New("not connected: run leah connect first")
	}
	return string(t), nil
}

type slackToken struct{ staticToken }

func (s slackToken) BotToken(ctx context.Context) (string, error)  { return s.Token(ctx) }
func (s slackToken) UserToken(ctx context.Context) (string, error) { return s.Token(ctx) }

func staticTokenForName(name string) staticToken {
	buf, err := os.ReadFile(connect.DefaultTokenPath(name))
	if err != nil {
		return ""
	}
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if json.Unmarshal(buf, &tok) != nil {
		return ""
	}
	return staticToken(tok.AccessToken)
}
