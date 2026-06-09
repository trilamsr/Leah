package web

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/budget"
	"github.com/trilam/leah/internal/memory"
	"github.com/trilam/leah/internal/regattaclient"
)

type fakeRegatta struct{ items []regattaclient.Agent }

func (f *fakeRegatta) List(context.Context) ([]regattaclient.Agent, error) { return f.items, nil }

func newTestServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()

	logger := &audit.Logger{Path: filepath.Join(dir, "audit.jsonl")}
	for i := 0; i < 3; i++ {
		if err := logger.Append(audit.Entry{Kind: "test.kind", Outcome: "ok", Detail: "row"}); err != nil {
			t.Fatalf("audit append: %v", err)
		}
	}

	store, err := memory.NewStore(filepath.Join(dir, "memory.db"))
	if err != nil {
		t.Fatalf("memory store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := store.AddContact(memory.Contact{Name: "alice"}); err != nil {
		t.Fatalf("add contact: %v", err)
	}
	if _, err := store.AddProject(memory.Project{Name: "leah"}); err != nil {
		t.Fatalf("add project: %v", err)
	}
	if _, err := store.AddDecision(memory.Decision{Topic: "stack", Choice: "vanilla"}); err != nil {
		t.Fatalf("add decision: %v", err)
	}

	b := &budget.Budget{Ceiling: 25.0}
	_ = b.Charge(1.23)

	hb := time.Now().Add(-15 * time.Second)
	return &Server{
		Addr:      "127.0.0.1:0",
		AuditPath: logger.Path,
		Memory:    store,
		Regatta:   &fakeRegatta{items: []regattaclient.Agent{{ID: "agent-1", Branch: "regatta/agent-1", State: "running", PR: 99}}},
		Budget:    b,
		StartTime: time.Now().Add(-time.Hour),
		Heartbeat: func() time.Time { return hb },
	}
}

// TestSnapshotAggregatesFromAllSources verifies the State struct pulls
// expected values from audit log + memory store + regatta lister + budget +
// start time + heartbeat function.
func TestSnapshotAggregatesFromAllSources(t *testing.T) {
	s := newTestServer(t)
	state := s.Snapshot(context.Background())

	if len(state.Audit) != 3 {
		t.Errorf("audit rows: got %d, want 3", len(state.Audit))
	}
	if len(state.Agents) != 1 || state.Agents[0].ID != "agent-1" {
		t.Errorf("agents: got %+v", state.Agents)
	}
	if state.Memory.Contacts != 1 || state.Memory.Projects != 1 || state.Memory.Decisions != 1 {
		t.Errorf("memory counts: got %+v", state.Memory)
	}
	if len(state.Memory.RecentDecisions) != 1 || state.Memory.RecentDecisions[0].Topic != "stack" {
		t.Errorf("recent decisions: got %+v", state.Memory.RecentDecisions)
	}
	if state.Ops.BudgetSpent < 1.23 || state.Ops.BudgetCeiling != 25.0 {
		t.Errorf("budget: got spent=%v ceiling=%v", state.Ops.BudgetSpent, state.Ops.BudgetCeiling)
	}
	if state.Ops.DaemonUptimeSeconds < 3500 {
		t.Errorf("uptime: got %d, want >=3500", state.Ops.DaemonUptimeSeconds)
	}
	if state.Ops.LastHeartbeatAt == "" {
		t.Error("heartbeat: empty")
	}
}

// TestHandleStateReturnsJSON checks the /api/state handler returns 200 +
// valid JSON of the expected shape.
func TestHandleStateReturnsJSON(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/state", nil)
	rec := httptest.NewRecorder()
	s.handleState(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type: got %q", ct)
	}
	var got State
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Memory.Contacts != 1 {
		t.Errorf("decoded contacts: got %d", got.Memory.Contacts)
	}
}

// TestEnforceLoopbackRejectsNonLoopback asserts the bind guard refuses
// 0.0.0.0 / public addresses; accepts 127.0.0.1, localhost, ::1.
func TestEnforceLoopbackRejectsNonLoopback(t *testing.T) {
	good := []string{"127.0.0.1:8080", "localhost:8080", "[::1]:8080"}
	bad := []string{"0.0.0.0:8080", "10.0.0.1:8080", "example.com:8080"}
	for _, a := range good {
		if err := enforceLoopback(a); err != nil {
			t.Errorf("enforceLoopback(%q) = %v, want nil", a, err)
		}
	}
	for _, a := range bad {
		if err := enforceLoopback(a); err == nil {
			t.Errorf("enforceLoopback(%q) = nil, want error", a)
		}
	}
}

// TestServerStartsAndStops asserts Start binds, serves /api/state, then
// returns nil after ctx cancellation (graceful shutdown).
func TestServerStartsAndStops(t *testing.T) {
	s := newTestServer(t)
	s.Addr = pickPort(t)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- s.Start(ctx) }()

	deadline := time.Now().Add(2 * time.Second)
	var ok bool
	for time.Now().Before(deadline) {
		resp, err := http.Get("http://" + s.Addr + "/api/state")
		if err == nil {
			_ = resp.Body.Close()
			ok = (resp.StatusCode == http.StatusOK)
			break
		}
		time.Sleep(20 * time.Millisecond) // allow-sleep: poll for HTTP bind, not assertion wait
	}
	if !ok {
		t.Error("server never served /api/state")
	}
	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Start returned %v, want nil after cancel", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("Start did not return after ctx cancel")
	}
}

// TestDashboardServesHTML asserts /dashboard returns the embedded HTML page.
func TestDashboardServesHTML(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	rec := httptest.NewRecorder()
	s.handleDashboard(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("content-type: got %q", ct)
	}
	if !strings.Contains(rec.Body.String(), "dashboard") {
		t.Error("body missing 'dashboard' marker")
	}
}

func pickPort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("pick port: %v", err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	return addr
}
