// Package web serves the JARVIS-aesthetic personal-use dashboard.
// Single endpoint (/api/state), single page (/dashboard). See
// docs/specs/2026-06-09-jarvis-ui.md.
package web

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"time"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/budget"
	"github.com/trilam/leah/internal/memory"
	"github.com/trilam/leah/internal/regattaclient"
)

// State is the single JSON shape returned by /api/state. Mirrors spec §4.
type State struct {
	Audit  []AuditRow `json:"audit"`
	Agents []Agent    `json:"agents"`
	Memory MemoryView `json:"memory"`
	Ops    OpsView    `json:"ops"`
}

// AuditRow is one entry surfaced from audit.jsonl tail.
type AuditRow struct {
	TS          string `json:"ts"`
	Kind        string `json:"kind"`
	BlastRadius int    `json:"blast_radius"`
	Outcome     string `json:"outcome"`
	Detail      string `json:"detail,omitempty"`
}

// Agent is one regatta agent row.
type Agent struct {
	ID     string `json:"id"`
	Branch string `json:"branch"`
	State  string `json:"state"`
	PR     int    `json:"pr"`
}

// MemoryView shows memory KB cardinality plus recent decisions.
type MemoryView struct {
	Contacts         int        `json:"contacts"`
	Projects         int        `json:"projects"`
	Decisions        int        `json:"decisions"`
	RecentDecisions  []Decision `json:"recent_decisions"`
}

// Decision is a memory.Decision projection (top-N recent).
type Decision struct {
	Topic     string `json:"topic"`
	Choice    string `json:"choice"`
	DecidedAt string `json:"decided_at"`
}

// OpsView captures daemon vitals + budget gauge.
type OpsView struct {
	BudgetSpent         float64 `json:"budget_spent"`
	BudgetCeiling       float64 `json:"budget_ceiling"`
	DaemonUptimeSeconds int64   `json:"daemon_uptime_seconds"`
	LastHeartbeatAt     string  `json:"last_heartbeat_at"`
}

// RegattaLister is the subset of regattaclient.Client the web layer needs.
// Defined here so tests can stub without spawning the regatta binary.
type RegattaLister interface {
	List(ctx context.Context) ([]regattaclient.Agent, error)
}

// Snapshot aggregates current state from all sources. Errors from individual
// sources degrade to empty slices / zero values — dashboard must never 500
// because regatta is offline.
func (s *Server) Snapshot(ctx context.Context) State {
	out := State{
		Audit:  tailAudit(s.AuditPath, 20),
		Agents: listAgents(ctx, s.Regatta),
		Memory: readMemory(s.Memory),
		Ops:    s.readOps(),
	}
	return out
}

func tailAudit(path string, n int) []AuditRow {
	if path == "" {
		return []AuditRow{}
	}
	f, err := os.Open(path)
	if err != nil {
		return []AuditRow{}
	}
	defer func() { _ = f.Close() }()
	// Read all, keep last n. audit.jsonl is small (rotated by operator); a
	// full scan is fine vs a reverse-seek implementation.
	var rows []AuditRow
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		var e audit.Entry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			continue
		}
		rows = append(rows, AuditRow{
			TS:          e.Timestamp,
			Kind:        e.Kind,
			BlastRadius: e.BlastRadius,
			Outcome:     e.Outcome,
			Detail:      e.Detail,
		})
	}
	if len(rows) > n {
		rows = rows[len(rows)-n:]
	}
	if rows == nil {
		rows = []AuditRow{}
	}
	return rows
}

func listAgents(ctx context.Context, r RegattaLister) []Agent {
	if r == nil {
		return []Agent{}
	}
	cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	a, err := r.List(cctx)
	if err != nil {
		return []Agent{}
	}
	out := make([]Agent, 0, len(a))
	for _, x := range a {
		out = append(out, Agent{ID: x.ID, Branch: x.Branch, State: x.State, PR: x.PR})
	}
	return out
}

func readMemory(m *memory.Store) MemoryView {
	mv := MemoryView{RecentDecisions: []Decision{}}
	if m == nil {
		return mv
	}
	if c, err := m.ListContacts(); err == nil {
		mv.Contacts = len(c)
	}
	if p, err := m.ListProjects(); err == nil {
		mv.Projects = len(p)
	}
	if d, err := m.ListDecisions(); err == nil {
		mv.Decisions = len(d)
		top := d
		if len(top) > 5 {
			top = top[:5]
		}
		for _, x := range top {
			mv.RecentDecisions = append(mv.RecentDecisions, Decision{
				Topic: x.Topic, Choice: x.Choice, DecidedAt: x.DecidedAt,
			})
		}
	}
	return mv
}

func (s *Server) readOps() OpsView {
	v := OpsView{}
	if s.Budget != nil {
		v.BudgetSpent = s.Budget.Spent()
		v.BudgetCeiling = s.Budget.Ceiling
	} else {
		v.BudgetCeiling = budget.DefaultCeiling
	}
	if !s.StartTime.IsZero() {
		v.DaemonUptimeSeconds = int64(time.Since(s.StartTime).Seconds())
	}
	if s.Heartbeat != nil {
		if hb := s.Heartbeat(); !hb.IsZero() {
			v.LastHeartbeatAt = hb.UTC().Format(time.RFC3339)
		}
	}
	return v
}
