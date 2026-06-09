package daemonloop

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/regattaclient"
)

type RegattaClient interface {
	List(ctx context.Context) ([]regattaclient.Agent, error)
}

type Heartbeat interface {
	Ping(ctx context.Context) error
}

type Notifier interface {
	Notify(ctx context.Context, title, body string) error
}

// Loop polls regatta state every PollEvery and notifies on terminal-state
// transitions. Single-goroutine; in-memory diff; cold-start does not notify.
type Loop struct {
	Regatta   RegattaClient
	Heartbeat Heartbeat
	Notify    Notifier
	Audit     *audit.Logger
	Out       io.Writer
	PollEvery time.Duration

	prevState map[string]string
	cold      bool
}

func New(rc RegattaClient, hb Heartbeat, nf Notifier, a *audit.Logger, out io.Writer, pollEvery time.Duration) *Loop {
	return &Loop{
		Regatta:   rc,
		Heartbeat: hb,
		Notify:    nf,
		Audit:     a,
		Out:       out,
		PollEvery: pollEvery,
		prevState: map[string]string{},
		cold:      true,
	}
}

// Run blocks until ctx is canceled.
func (l *Loop) Run(ctx context.Context) error {
	_, _ = fmt.Fprintln(l.Out, "leah-daemon: starting; poll interval:", l.PollEvery)
	for {
		l.tick(ctx)
		select {
		case <-ctx.Done():
			_, _ = fmt.Fprintln(l.Out, "leah-daemon: shutdown")
			return nil
		case <-time.After(l.PollEvery):
		}
	}
}

func (l *Loop) tick(ctx context.Context) {
	if l.Heartbeat != nil {
		_ = l.Heartbeat.Ping(ctx)
	}
	agents, err := l.Regatta.List(ctx)
	if err != nil {
		_, _ = fmt.Fprintf(l.Out, "leah-daemon: regatta list error: %v\n", err)
		return
	}
	current := map[string]string{}
	for _, a := range agents {
		current[a.ID] = a.State
	}

	if !l.cold {
		for id, newState := range current {
			oldState, existed := l.prevState[id]
			if existed && oldState != newState && isTerminal(newState) {
				l.notifyTransition(ctx, id, oldState, newState, agents)
			}
		}
	}

	l.prevState = current
	l.cold = false
}

func isTerminal(state string) bool {
	switch state {
	case "merged", "escalated", "failed", "stuck", "killed":
		return true
	}
	return false
}

func (l *Loop) notifyTransition(ctx context.Context, id, from, to string, agents []regattaclient.Agent) {
	var pr int
	for _, a := range agents {
		if a.ID == id {
			pr = a.PR
			break
		}
	}
	title := "Leah"
	body := fmt.Sprintf("agent %s: %s → %s (PR #%d)", id, from, to, pr)
	_ = l.Notify.Notify(ctx, title, body)
	_ = l.Audit.Append(audit.Entry{
		Kind:        "daemon.transition",
		BlastRadius: 0,
		Outcome:     "observed",
		Detail:      body,
	})
	_, _ = fmt.Fprintln(l.Out, body)
}
