package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/obs"
	"github.com/trilam/leah/internal/regattaclient"
)

func resetRegattaLogOnce() { regattaLogOnce = &sync.Once{} }

type stubAttestor struct{ err error }

func (s stubAttestor) Attest(context.Context, string) error { return s.err }

type stubAuditSink struct {
	mu   sync.Mutex
	rows []regattaclient.AuditRow
}

func (s *stubAuditSink) Record(row regattaclient.AuditRow) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rows = append(s.rows, row)
}

func TestBootRegatta_DetectSuccess_StoresGated(t *testing.T) {
	resetRegattaLogOnce()
	var buf bytes.Buffer
	reg := obs.NewRegistry()
	detect := func(context.Context, regattaclient.DetectOpts) (regattaclient.Mode, regattaclient.Config, error) {
		return regattaclient.ModeDocker, regattaclient.Config{URL: "http://127.0.0.1:9090"}, nil
	}
	gated, err := bootRegatta(context.Background(), bootRegattaOpts{
		Detect:   detect,
		Attestor: stubAttestor{},
		Audit:    &stubAuditSink{},
		Logger:   &buf,
		Registry: reg,
	})
	if err != nil {
		t.Fatalf("bootRegatta err = %v, want nil", err)
	}
	if gated == nil {
		t.Fatal("bootRegatta returned nil GatedClient on success — want non-nil")
	}
	st, err := gated.Status(context.Background())
	if err != nil {
		t.Fatalf("Status err = %v", err)
	}
	if st.Mode != string(regattaclient.ModeDocker) {
		t.Fatalf("Status.Mode = %q, want %q", st.Mode, regattaclient.ModeDocker)
	}
}

func TestBootRegatta_DetectNoMode_GracefulSkip(t *testing.T) {
	resetRegattaLogOnce()
	var buf bytes.Buffer
	reg := obs.NewRegistry()
	detect := func(context.Context, regattaclient.DetectOpts) (regattaclient.Mode, regattaclient.Config, error) {
		return regattaclient.ModeNone, regattaclient.Config{}, regattaclient.ErrNoMode
	}
	gated, err := bootRegatta(context.Background(), bootRegattaOpts{
		Detect:   detect,
		Attestor: stubAttestor{},
		Audit:    &stubAuditSink{},
		Logger:   &buf,
		Registry: reg,
	})
	if err != nil {
		t.Fatalf("bootRegatta err = %v, want nil (graceful skip)", err)
	}
	if gated != nil {
		t.Fatalf("bootRegatta returned non-nil GatedClient on ModeNone — want nil")
	}
	if !strings.Contains(buf.String(), "leah connect regatta") {
		t.Fatalf("missing connect-regatta hint in log: %q", buf.String())
	}
	if got := readGauge(t, reg, "leah_regatta_unavailable"); got != 1 {
		t.Fatalf("leah_regatta_unavailable = %v, want 1", got)
	}
}

func TestBootRegatta_DetectError_LogsOnce(t *testing.T) {
	resetRegattaLogOnce()
	var buf bytes.Buffer
	reg := obs.NewRegistry()
	probeErr := errors.New("docker probe blew up")
	detect := func(context.Context, regattaclient.DetectOpts) (regattaclient.Mode, regattaclient.Config, error) {
		return regattaclient.ModeNone, regattaclient.Config{}, probeErr
	}
	gated, err := bootRegatta(context.Background(), bootRegattaOpts{
		Detect:   detect,
		Attestor: stubAttestor{},
		Audit:    &stubAuditSink{},
		Logger:   &buf,
		Registry: reg,
	})
	if !errors.Is(err, probeErr) {
		t.Fatalf("bootRegatta err = %v, want wraps %v", err, probeErr)
	}
	if gated != nil {
		t.Fatalf("gated must be nil on detect error")
	}
	if !strings.Contains(buf.String(), "docker probe blew up") {
		t.Fatalf("missing probe error in log: %q", buf.String())
	}
	prev := buf.String()
	_, _ = bootRegatta(context.Background(), bootRegattaOpts{
		Detect:   detect,
		Attestor: stubAttestor{},
		Audit:    &stubAuditSink{},
		Logger:   &buf,
		Registry: reg,
	})
	if buf.String() != prev {
		t.Fatalf("log line emitted twice — want once.\nbefore: %q\nafter:  %q", prev, buf.String())
	}
}

func TestBootRegatta_GatedWiredAsRegatta(t *testing.T) {
	resetRegattaLogOnce()
	var buf bytes.Buffer
	reg := obs.NewRegistry()
	denyErr := errors.New("operator said no")
	sink := &stubAuditSink{}
	detect := func(context.Context, regattaclient.DetectOpts) (regattaclient.Mode, regattaclient.Config, error) {
		return regattaclient.ModeDocker, regattaclient.Config{URL: "http://127.0.0.1:9090"}, nil
	}
	gated, err := bootRegatta(context.Background(), bootRegattaOpts{
		Detect:   detect,
		Attestor: stubAttestor{err: denyErr},
		Audit:    sink,
		Logger:   &buf,
		Registry: reg,
	})
	if err != nil {
		t.Fatalf("bootRegatta err = %v", err)
	}
	if gated == nil {
		t.Fatal("gated nil")
	}
	if _, err := gated.Ship(context.Background(), regattaclient.ShipRequest{Branch: "x"}); !errors.Is(err, denyErr) {
		t.Fatalf("Ship err = %v, want wraps %v", err, denyErr)
	}
	sink.mu.Lock()
	rows := append([]regattaclient.AuditRow(nil), sink.rows...)
	sink.mu.Unlock()
	if len(rows) != 0 {
		t.Fatalf("denied attestation must not emit audit row, got %+v", rows)
	}
}

func TestAuditLoggerSink_RecordsKindAndOutcome(t *testing.T) {
	dir := t.TempDir()
	logger := &audit.Logger{Path: filepath.Join(dir, "audit.jsonl")}
	sink := newAuditLoggerSink(logger)
	sink.Record(regattaclient.AuditRow{Kind: "regatta_ship", Success: true})
	sink.Record(regattaclient.AuditRow{Kind: "regatta_review", Success: false})

	data, err := os.ReadFile(logger.Path)
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	var got []audit.Entry
	for _, line := range bytes.Split(bytes.TrimRight(data, "\n"), []byte("\n")) {
		var e audit.Entry
		if err := json.Unmarshal(line, &e); err != nil {
			t.Fatalf("decode: %v", err)
		}
		got = append(got, e)
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2", len(got))
	}
	if got[0].Kind != "regatta_ship" || got[0].Outcome != "success" {
		t.Fatalf("row 0 = %+v", got[0])
	}
	if got[1].Kind != "regatta_review" || got[1].Outcome != "failure" {
		t.Fatalf("row 1 = %+v", got[1])
	}
}

func TestDaemonRegattaInner_WritesRefuseUntilTransport(t *testing.T) {
	inner := newDaemonRegattaInner(regattaclient.ModeDocker, regattaclient.Config{})
	if _, err := inner.Ship(context.Background(), regattaclient.ShipRequest{}); !errors.Is(err, ErrRegattaNotConnected) {
		t.Fatalf("Ship err = %v, want ErrRegattaNotConnected", err)
	}
	if _, err := inner.Review(context.Background(), regattaclient.ReviewRequest{}); !errors.Is(err, ErrRegattaNotConnected) {
		t.Fatalf("Review err = %v, want ErrRegattaNotConnected", err)
	}
	st, err := inner.Status(context.Background())
	if err != nil {
		t.Fatalf("Status err = %v", err)
	}
	if !st.Healthy || st.Mode != string(regattaclient.ModeDocker) {
		t.Fatalf("Status = %+v", st)
	}
}

// readGauge snapshots the registry and pulls the named gauge's empty-label
// value. Goes through Snapshot() to avoid touching unexported state.
func readGauge(t *testing.T, r *obs.Registry, name string) float64 {
	t.Helper()
	path := filepath.Join(t.TempDir(), "snap.json")
	if err := r.Snapshot(path); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read snap: %v", err)
	}
	var snap struct {
		Gauges map[string]float64 `json:"gauges"`
	}
	if err := json.Unmarshal(data, &snap); err != nil {
		t.Fatalf("decode snap: %v", err)
	}
	// Empty-label gauges land under the bare metric name (per obs_test.go).
	if v, ok := snap.Gauges[name]; ok {
		return v
	}
	return 0
}
