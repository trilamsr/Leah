package web

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/budget"
	"github.com/trilam/leah/internal/memory"
	"github.com/trilam/leah/internal/obs"
	"github.com/trilam/leah/internal/regattaclient"
)

type fakeRegatta struct{ items []regattaclient.Agent }

func (f *fakeRegatta) List(context.Context) ([]regattaclient.Agent, error) { return f.items, nil }

func newTestServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()

	logger := &audit.Logger{Path: filepath.Join(dir, "audit.jsonl")}
	for i := 0; i < 3; i++ {
		if err := logger.Append(audit.Entry{Kind: "test.kind", Outcome: "ok", Detail: "row", CostDollars: 0.10}); err != nil {
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

func TestSpamView_WiredProviderSurfacesStats(t *testing.T) {
	s := newTestServer(t)
	s.SpamStats = func() []SpamStat {
		return []SpamStat{{Adapter: "discord", Sends: 4, Denied: 1}}
	}
	v := s.Snapshot(context.Background()).Spam
	if len(v) != 1 || v[0].Adapter != "discord" || v[0].Sends != 4 || v[0].Denied != 1 {
		t.Errorf("Spam: got %+v, want [{discord 4 1}]", v)
	}
}

func TestSpamView_NilProviderDegradesToEmpty(t *testing.T) {
	s := newTestServer(t)
	if v := s.Snapshot(context.Background()).Spam; len(v) != 0 {
		t.Errorf("Spam with nil provider: got %+v, want empty", v)
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
	// Costs: three 0.10 audit rows written above with default time.Now()
	// should aggregate to 0.30 USD in the 24h + 7d windows.
	if got := state.Costs.WeekUSD; got < 0.29 || got > 0.31 {
		t.Errorf("Costs.WeekUSD: got %v, want ~0.30", got)
	}
	if got := state.Costs.TodayUSD; got < 0.29 || got > 0.31 {
		t.Errorf("Costs.TodayUSD: got %v, want ~0.30", got)
	}
	if len(state.Costs.TopKinds) == 0 || state.Costs.TopKinds[0].Name != "test.kind" {
		t.Errorf("Costs.TopKinds: got %+v", state.Costs.TopKinds)
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

// TestSnapshotCachesWithinTTL asserts Snapshot serves a cached State when
// called within CacheTTL of the prior call — collapses the 3s dashboard
// poll's audit-scan + sqlite + metrics-read cost to one full computation
// per TTL window (H4 from Wave2-5 retro audit).
func TestSnapshotCachesWithinTTL(t *testing.T) {
	s := newTestServer(t)
	s.CacheTTL = 10 * time.Second

	first := s.Snapshot(context.Background())
	// Append an audit row AFTER the first Snapshot — within TTL, the second
	// call should NOT see it because the cached State is reused.
	logger := &audit.Logger{Path: s.AuditPath}
	if err := logger.Append(audit.Entry{Kind: "later.kind", Outcome: "ok"}); err != nil {
		t.Fatalf("audit append: %v", err)
	}
	second := s.Snapshot(context.Background())
	if len(second.Audit) != len(first.Audit) {
		t.Errorf("expected cached State (len=%d), got fresh (len=%d)", len(first.Audit), len(second.Audit))
	}
}

// TestSnapshotRefreshesAfterTTL asserts the cache is bypassed once TTL
// elapses — set CacheTTL to a small positive value, sleep past it, expect
// the new audit row to surface.
func TestSnapshotRefreshesAfterTTL(t *testing.T) {
	s := newTestServer(t)
	s.CacheTTL = 5 * time.Millisecond

	first := s.Snapshot(context.Background())
	logger := &audit.Logger{Path: s.AuditPath}
	if err := logger.Append(audit.Entry{Kind: "later.kind", Outcome: "ok"}); err != nil {
		t.Fatalf("audit append: %v", err)
	}
	time.Sleep(20 * time.Millisecond) // allow-sleep: TTL expiry, not assertion wait
	second := s.Snapshot(context.Background())
	if len(second.Audit) <= len(first.Audit) {
		t.Errorf("expected refreshed State (len>%d), got cached (len=%d)", len(first.Audit), len(second.Audit))
	}
}

// TestSSE_HUDStateEvent_ShapeMatchesJSContract asserts the hud.state
// Payload exposes value/listening/thinking — the exact fields ambient.js
// reads to drive the state pill + listen/think rings. Pre-V2 the payload
// was web.State{} which has none of these → the pill froze at "ambient"
// whenever the daemon /events relay was up.
func TestSSE_HUDStateEvent_ShapeMatchesJSContract(t *testing.T) {
	e := computeHUDStateEvent("focus", true, false)
	raw, err := json.Marshal(e.Payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	js := string(raw)
	for _, want := range []string{`"value"`, `"listening"`, `"thinking"`} {
		if !strings.Contains(js, want) {
			t.Errorf("payload missing field %s: %s", want, js)
		}
	}
	if !strings.Contains(js, `"value":"focus"`) {
		t.Errorf("payload value mismatch: %s", js)
	}
	if !strings.Contains(js, `"listening":true`) {
		t.Errorf("payload listening mismatch: %s", js)
	}
}

// TestSnapshotConcurrentReadsNoRace exercises concurrent Snapshot calls
// under -race to assert the cache + RWMutex are safe.
func TestSnapshotConcurrentReadsNoRace(t *testing.T) {
	s := newTestServer(t)
	s.CacheTTL = 50 * time.Millisecond

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				_ = s.Snapshot(context.Background())
			}
		}()
	}
	wg.Wait()
}

// mux returns the wired-up handler graph for httptest assertions, panicking
// on the embed-fs error path (which is impossible at runtime — the embed
// directive is checked at compile time).
func (s *Server) mux() http.Handler {
	m, err := s.buildMux()
	if err != nil {
		panic(err)
	}
	return m
}

// TestDashboardRedirectsToStatic asserts /dashboard 301-redirects to
// /static/dashboard.html so the embed FS file server is the sole serving
// path (dedup: handleDashboard's per-request ReadFile is gone).
func TestDashboardRedirectsToStatic(t *testing.T) {
	mux := newTestServer(t).mux()
	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("status: got %d, want 301", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/static/dashboard.html" {
		t.Errorf("Location: got %q, want /static/dashboard.html", loc)
	}
}

// TestStaticDashboardServesHTML asserts /static/dashboard.html serves the
// embedded page directly via the file server (post-dedup destination).
func TestStaticDashboardServesHTML(t *testing.T) {
	mux := newTestServer(t).mux()
	req := httptest.NewRequest(http.MethodGet, "/static/dashboard.html", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
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

// TestMemoryView_RecentDecisions_RespectsLimit asserts the cap is the named constant.
func TestMemoryView_RecentDecisions_RespectsLimit(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewStore(filepath.Join(dir, "memory.db"))
	if err != nil {
		t.Fatalf("memory store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	for i := 0; i < RecentDecisionsLimit+3; i++ {
		if _, err := store.AddDecision(memory.Decision{Topic: "t", Choice: "c"}); err != nil {
			t.Fatalf("add decision: %v", err)
		}
	}
	mv := readMemory(store)
	if got := len(mv.RecentDecisions); got != RecentDecisionsLimit {
		t.Errorf("RecentDecisions len: got %d, want %d", got, RecentDecisionsLimit)
	}
}

// TestMemoryView_RecentDecisions_FewerThanLimit asserts no padding when source is short.
func TestMemoryView_RecentDecisions_FewerThanLimit(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewStore(filepath.Join(dir, "memory.db"))
	if err != nil {
		t.Fatalf("memory store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	want := RecentDecisionsLimit - 2
	if want < 1 {
		want = 1
	}
	for i := 0; i < want; i++ {
		if _, err := store.AddDecision(memory.Decision{Topic: "t", Choice: "c"}); err != nil {
			t.Fatalf("add decision: %v", err)
		}
	}
	mv := readMemory(store)
	if got := len(mv.RecentDecisions); got != want {
		t.Errorf("RecentDecisions len: got %d, want %d", got, want)
	}
}

// TestTruncateForTooltip_Short asserts strings within max pass through unchanged.
func TestTruncateForTooltip_Short(t *testing.T) {
	in := "short"
	display, tooltip := truncateForTooltip(in, 80)
	if display != in || tooltip != in {
		t.Errorf("short: got (%q,%q), want (%q,%q)", display, tooltip, in, in)
	}
}

// TestTruncateForTooltip_Long asserts long strings get ellipsis display + full tooltip.
func TestTruncateForTooltip_Long(t *testing.T) {
	in := strings.Repeat("x", 200)
	display, tooltip := truncateForTooltip(in, 80)
	if tooltip != in {
		t.Errorf("tooltip: not full input")
	}
	if !strings.HasSuffix(display, "…") {
		t.Errorf("display: missing ellipsis, got %q", display)
	}
	if len([]rune(display)) != 80 {
		t.Errorf("display rune-len: got %d, want 80", len([]rune(display)))
	}
}

// TestTruncateForTooltip_ExactMax asserts boundary input does not get truncated.
func TestTruncateForTooltip_ExactMax(t *testing.T) {
	in := strings.Repeat("y", 80)
	display, tooltip := truncateForTooltip(in, 80)
	if display != in || tooltip != in {
		t.Errorf("boundary: got (%q,%q), want unchanged", display, tooltip)
	}
}

// auditParseErrorCount reads the counter value under the audit_jsonl source
// label so tests can assert tailAudit's silent-failure replacement (BB-RETRO M2, #5).
func auditParseErrorCount(t *testing.T, reg *obs.Registry) int64 {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "snap.json")
	if err := reg.Snapshot(path); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read snap: %v", err)
	}
	var snap struct {
		Counters map[string]int64 `json:"counters"`
	}
	if err := json.Unmarshal(raw, &snap); err != nil {
		t.Fatalf("decode snap: %v", err)
	}
	for k, v := range snap.Counters {
		if strings.HasPrefix(k, "leah_audit_parse_errors_total") {
			return v
		}
	}
	return 0
}

// TestTailAudit_IncrementsCounterOnBadJSON feeds mixed valid+malformed lines and asserts the counter rises by the malformed count.
func TestTailAudit_IncrementsCounterOnBadJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	body := `{"kind":"ok1","outcome":"ok"}
not-json-line-1
{"kind":"ok2","outcome":"ok"}
{broken json
also bad
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write audit: %v", err)
	}
	reg := obs.NewRegistry()
	s := &Server{AuditPath: path, Metrics: reg}
	rows := s.tailAudit(20)
	if len(rows) != 2 {
		t.Errorf("rows: got %d, want 2", len(rows))
	}
	if got := auditParseErrorCount(t, reg); got != 3 {
		t.Errorf("parse-error counter: got %d, want 3", got)
	}
}

// TestTailAudit_CleanLogNoIncrement asserts a fully valid log leaves the counter at zero.
func TestTailAudit_CleanLogNoIncrement(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	body := `{"kind":"ok1","outcome":"ok"}
{"kind":"ok2","outcome":"ok"}
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write audit: %v", err)
	}
	reg := obs.NewRegistry()
	s := &Server{AuditPath: path, Metrics: reg}
	rows := s.tailAudit(20)
	if len(rows) != 2 {
		t.Errorf("rows: got %d, want 2", len(rows))
	}
	if got := auditParseErrorCount(t, reg); got != 0 {
		t.Errorf("parse-error counter: got %d, want 0", got)
	}
}

// TestTailAudit_ContinuesAfterError asserts malformed lines never abort the scan.
func TestTailAudit_ContinuesAfterError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	body := `garbage
{"kind":"good1","outcome":"ok"}
more garbage
{"kind":"good2","outcome":"ok"}
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write audit: %v", err)
	}
	reg := obs.NewRegistry()
	s := &Server{AuditPath: path, Metrics: reg}
	rows := s.tailAudit(20)
	if len(rows) != 2 {
		t.Errorf("rows: got %d, want 2 (parse must not abort)", len(rows))
	}
	if rows[0].Kind != "good1" || rows[1].Kind != "good2" {
		t.Errorf("rows order: got %+v", rows)
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
