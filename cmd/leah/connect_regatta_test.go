package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/trilam/leah/internal/connect"
)

type tFakeExec struct {
	mu     sync.Mutex
	calls  [][]string
	script []tFakeResp
	idx    int
}

type tFakeResp struct {
	stdout []byte
	stderr []byte
	err    error
}

func (f *tFakeExec) Run(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	argv := append([]string{name}, args...)
	f.calls = append(f.calls, argv)
	if f.idx >= len(f.script) {
		return nil, nil, errors.New("unscripted")
	}
	r := f.script[f.idx]
	f.idx++
	return r.stdout, r.stderr, r.err
}

// TestRunConnectRegatta_Help short-circuits with exit 0 before any state work
// — mirrors the connect.go help-arg contract.
func TestRunConnectRegatta_Help(t *testing.T) {
	var buf bytes.Buffer
	if code := runConnectRegatta(context.Background(), []string{"--help"}, &buf, nil); code != 0 {
		t.Fatalf("exit %d, want 0", code)
	}
}

// TestRunConnectRegatta_HappyPath wires the CLI handler to a fake exec + a
// healthz httptest. Asserts exit=0, the mode file lands at the expected path,
// and exactly one success audit row is appended.
func TestRunConnectRegatta_HappyPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	t.Setenv("LEAH_CONNECT_AUTO_ATTEST", "1")

	hz := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer hz.Close()

	fx := &tFakeExec{script: []tFakeResp{
		{stdout: []byte("ok")}, {stdout: []byte("ok")}, {stdout: []byte("ok")},
	}}
	p := &connect.RegattaProvider{
		Exec:         fx,
		HealthzURL:   hz.URL + "/healthz",
		HealthzPoll:  5 * time.Millisecond,
		HealthBudget: 500 * time.Millisecond,
	}

	var buf bytes.Buffer
	if code := runConnectRegatta(context.Background(), []string{}, &buf, p); code != 0 {
		t.Fatalf("exit %d, out=%s", code, buf.String())
	}
	if !strings.Contains(buf.String(), "ok: regatta") {
		t.Fatalf("ok line missing: %s", buf.String())
	}
	modePath := filepath.Join(dir, "secrets", "regatta-mode.json")
	if _, err := os.Stat(modePath); err != nil {
		t.Fatalf("mode file missing: %v", err)
	}

	// Audit row appended.
	auditRaw, err := os.ReadFile(filepath.Join(dir, "audit.jsonl"))
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	if !bytes.Contains(auditRaw, []byte(`"connect_regatta_docker"`)) {
		t.Fatalf("audit kind missing: %s", auditRaw)
	}
	if !bytes.Contains(auditRaw, []byte(`"success"`)) {
		t.Fatalf("audit outcome success missing: %s", auditRaw)
	}
}

// TestRunConnectRegatta_DockerMissing surfaces ErrDockerUnavailable as a
// non-zero exit + a remediation hint on stderr; no mode file written.
func TestRunConnectRegatta_DockerMissing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	t.Setenv("LEAH_CONNECT_AUTO_ATTEST", "1")

	fx := &tFakeExec{script: []tFakeResp{
		{stderr: []byte("Cannot connect to the Docker daemon"), err: errors.New("exit 1")},
	}}
	p := &connect.RegattaProvider{Exec: fx}

	var buf bytes.Buffer
	if code := runConnectRegatta(context.Background(), []string{}, &buf, p); code == 0 {
		t.Fatalf("expected non-zero exit on docker missing")
	}
	if _, err := os.Stat(filepath.Join(dir, "secrets", "regatta-mode.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("mode file leaked")
	}
	// Audit row records the failure.
	auditRaw, err := os.ReadFile(filepath.Join(dir, "audit.jsonl"))
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	var row map[string]any
	for _, line := range bytes.Split(bytes.TrimSpace(auditRaw), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		if err := json.Unmarshal(line, &row); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
	}
	if row["kind"] != "connect_regatta_docker" || row["outcome"] != "failed" {
		t.Fatalf("audit row mismatch: %v", row)
	}
}
