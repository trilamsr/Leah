package regattaclient

import (
	"context"
	"strings"
	"testing"
)

type fakeExec struct {
	calls   [][]string
	respOut string
}

func (f *fakeExec) Run(ctx context.Context, args []string) (string, error) {
	f.calls = append(f.calls, args)
	return f.respOut, nil
}

func TestListAgentsParsesJSON(t *testing.T) {
	fe := &fakeExec{respOut: `[
		{"id":"a1","branch":"regatta/agent-1","state":"running","pr":1112},
		{"id":"a2","branch":"regatta/agent-2","state":"escalated","pr":null}
	]`}
	c := &Client{Exec: fe}

	agents, err := c.List(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(agents) != 2 {
		t.Fatalf("want 2 agents, got %d", len(agents))
	}
	if agents[0].ID != "a1" || agents[0].State != "running" || agents[0].PR != 1112 {
		t.Errorf("agent[0] mismatch: %+v", agents[0])
	}
	if agents[1].State != "escalated" {
		t.Errorf("agent[1] state: %v", agents[1].State)
	}
	if !strings.Contains(strings.Join(fe.calls[0], " "), "agents list --json") {
		t.Errorf("call: %v", fe.calls[0])
	}
}
