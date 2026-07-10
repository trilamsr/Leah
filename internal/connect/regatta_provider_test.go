package connect

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeExec records every os/exec call and returns scripted outputs in order
// of call. Tests assert on the recorded argv to prove the docker pipeline ran
// in the load-bearing order info → pull → run → (on failure) stop+rm.
type fakeExec struct {
	mu     sync.Mutex
	calls  [][]string
	script []fakeExecResp
	idx    int
}

type fakeExecResp struct {
	stdout []byte
	stderr []byte
	err    error
}

func (f *fakeExec) Run(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	argv := append([]string{name}, args...)
	f.calls = append(f.calls, argv)
	if f.idx >= len(f.script) {
		return nil, nil, fmt.Errorf("unscripted call: %v", argv)
	}
	r := f.script[f.idx]
	f.idx++
	return r.stdout, r.stderr, r.err
}

func (f *fakeExec) argvAt(i int) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if i >= len(f.calls) {
		return nil
	}
	return f.calls[i]
}

func (f *fakeExec) calledNames() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.calls))
	for i, c := range f.calls {
		if len(c) >= 2 {
			out[i] = c[0] + " " + c[1]
		} else if len(c) == 1 {
			out[i] = c[0]
		}
	}
	return out
}

type regattaFakeAttestor struct {
	allow bool
}

func (a regattaFakeAttestor) Attest(_ context.Context, _ string) error {
	if a.allow {
		return nil
	}
	return errors.New("denied")
}

type capturedAudit struct {
	mu      sync.Mutex
	entries []AuditEntry
}

func (c *capturedAudit) Record(e AuditEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = append(c.entries, e)
}

func (c *capturedAudit) only() AuditEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) != 1 {
		return AuditEntry{}
	}
	return c.entries[0]
}

// TestConnectRegattaDocker_HappyPath drives the entire docker pipeline through
// a fake Exec + an httptest healthz server. Assertions: docker argv order is
// info, pull, run; the -p binding is loopback-only; mode file is written 0600
// with mode == "docker"; one success audit row.
func TestConnectRegattaDocker_HappyPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)

	hz := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer hz.Close()

	fx := &fakeExec{script: []fakeExecResp{
		{stdout: []byte("ok")}, // info
		{stdout: []byte("ok")}, // pull
		{stdout: []byte("ok")}, // run
	}}
	audit := &capturedAudit{}
	p := &RegattaProvider{
		Exec:         fx,
		HealthzURL:   hz.URL + "/healthz",
		HealthzPoll:  5 * time.Millisecond,
		HealthBudget: 500 * time.Millisecond,
		Audit:        audit,
		Now:          func() time.Time { return time.Unix(1717977600, 0).UTC() },
	}

	if err := p.Connect(context.Background(), regattaFakeAttestor{allow: true}); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// argv order
	if got := fx.argvAt(0); len(got) < 2 || got[0] != "docker" || got[1] != "info" {
		t.Fatalf("call 0 not docker info: %v", got)
	}
	if got := fx.argvAt(1); len(got) < 2 || got[0] != "docker" || got[1] != "pull" {
		t.Fatalf("call 1 not docker pull: %v", got)
	}
	if got := fx.argvAt(2); len(got) < 2 || got[0] != "docker" || got[1] != "run" {
		t.Fatalf("call 2 not docker run: %v", got)
	}

	// Loopback-only port mapping.
	runArgs := fx.argvAt(2)
	joined := strings.Join(runArgs, " ")
	if !strings.Contains(joined, "127.0.0.1:9090:9090") {
		t.Fatalf("missing loopback port mapping: %v", runArgs)
	}
	if strings.Contains(joined, "0.0.0.0:9090") {
		t.Fatalf("found 0.0.0.0 binding (LAN-exposed): %v", runArgs)
	}
	// Pinned by digest, not :latest.
	if strings.Contains(joined, ":latest") {
		t.Fatalf("image pinned by :latest (must be @sha256:): %v", runArgs)
	}
	if !strings.Contains(joined, "@sha256:") {
		t.Fatalf("image not pinned by digest: %v", runArgs)
	}
	// Container name + named volume.
	if !strings.Contains(joined, "leah-regatta") {
		t.Fatalf("missing container name: %v", runArgs)
	}

	// Mode file written 0600 with correct payload.
	modePath := filepath.Join(dir, "secrets", "regatta-mode.json")
	st, err := os.Stat(modePath)
	if err != nil {
		t.Fatalf("stat mode file: %v", err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("mode file perm = %v, want 0600", st.Mode().Perm())
	}
	raw, err := os.ReadFile(modePath)
	if err != nil {
		t.Fatalf("read mode: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal mode: %v", err)
	}
	if doc["mode"] != "docker" {
		t.Fatalf("mode != docker: %v", doc)
	}
	if doc["container"] != "leah-regatta" {
		t.Fatalf("container != leah-regatta: %v", doc)
	}
	if doc["url"] == "" {
		t.Fatalf("missing url: %v", doc)
	}

	// Audit row.
	e := audit.only()
	if e.Kind != "connect_regatta_docker" {
		t.Fatalf("audit kind = %q", e.Kind)
	}
	if !e.Success {
		t.Fatalf("audit success false: %+v", e)
	}
}

// TestConnectRegattaDocker_AttestationDenied_NoDocker proves the consent gate
// fires BEFORE any os/exec. A denied attestor MUST short-circuit with zero
// docker invocations and no mode file on disk.
func TestConnectRegattaDocker_AttestationDenied_NoDocker(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)

	fx := &fakeExec{}
	audit := &capturedAudit{}
	p := &RegattaProvider{Exec: fx, Audit: audit}

	err := p.Connect(context.Background(), regattaFakeAttestor{allow: false})
	if !errors.Is(err, ErrAttestationDenied) {
		t.Fatalf("err = %v, want ErrAttestationDenied", err)
	}
	if len(fx.calls) != 0 {
		t.Fatalf("expected zero exec calls, got %v", fx.calls)
	}
	if _, err := os.Stat(filepath.Join(dir, "secrets", "regatta-mode.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("mode file leaked on denied attestation")
	}
	e := audit.only()
	if e.Success || e.Reason != "attestation_denied" {
		t.Fatalf("audit row mismatch: %+v", e)
	}
}

// TestConnectRegattaDocker_DockerMissing_ReturnsErrDockerUnavailable asserts
// that a failed `docker info` short-circuits before pull/run and surfaces a
// sentinel error the CLI can format with remediation text.
func TestConnectRegattaDocker_DockerMissing_ReturnsErrDockerUnavailable(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)

	fx := &fakeExec{script: []fakeExecResp{
		{stderr: []byte("Cannot connect to the Docker daemon"), err: errors.New("exit 1")},
	}}
	audit := &capturedAudit{}
	p := &RegattaProvider{Exec: fx, Audit: audit}

	err := p.Connect(context.Background(), regattaFakeAttestor{allow: true})
	if !errors.Is(err, ErrDockerUnavailable) {
		t.Fatalf("err = %v, want ErrDockerUnavailable", err)
	}
	// Only `docker info` ran.
	if len(fx.calls) != 1 {
		t.Fatalf("expected exactly 1 exec call (info), got %v", fx.calledNames())
	}
	if _, err := os.Stat(filepath.Join(dir, "secrets", "regatta-mode.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("mode file leaked on docker missing")
	}
}

// TestConnectRegattaDocker_HealthPollTimeout_TearsDownContainer asserts the
// "no orphan container" invariant — when the healthz budget exhausts after
// `docker run` succeeded, Connect MUST issue `docker stop` and `docker rm`
// before returning, and no mode file is written.
func TestConnectRegattaDocker_HealthPollTimeout_TearsDownContainer(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)

	// Health server that always 503s — forces the budget to exhaust.
	hz := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer hz.Close()

	fx := &fakeExec{script: []fakeExecResp{
		{stdout: []byte("ok")}, // info
		{stdout: []byte("ok")}, // pull
		{stdout: []byte("ok")}, // run
		{stdout: []byte("ok")}, // stop (teardown)
		{stdout: []byte("ok")}, // rm   (teardown)
	}}
	audit := &capturedAudit{}
	p := &RegattaProvider{
		Exec:         fx,
		HealthzURL:   hz.URL + "/healthz",
		HealthzPoll:  5 * time.Millisecond,
		HealthBudget: 30 * time.Millisecond, // tiny budget — exhausts fast
		Audit:        audit,
	}

	err := p.Connect(context.Background(), regattaFakeAttestor{allow: true})
	if err == nil {
		t.Fatalf("expected error on health timeout")
	}

	// Last two calls must be stop + rm against the container.
	if len(fx.calls) < 5 {
		t.Fatalf("expected teardown calls (5 total), got %d: %v", len(fx.calls), fx.calledNames())
	}
	stop := fx.argvAt(3)
	rm := fx.argvAt(4)
	if stop[0] != "docker" || stop[1] != "stop" || !contains(stop, "leah-regatta") {
		t.Fatalf("teardown stop wrong: %v", stop)
	}
	if rm[0] != "docker" || rm[1] != "rm" || !contains(rm, "leah-regatta") {
		t.Fatalf("teardown rm wrong: %v", rm)
	}

	if _, err := os.Stat(filepath.Join(dir, "secrets", "regatta-mode.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("mode file leaked on health timeout — partial state survives")
	}
}

// TestConnectRegattaDocker_ModeFileMode0600 is a re-assertion that the mode
// file mode is 0600 against a hostile pre-existing world-readable file. The
// happy-path test runs from an empty tempdir; this one pre-creates the file
// 0644 to force the explicit Chmod path.
func TestConnectRegattaDocker_ModeFileMode0600(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	secrets := filepath.Join(dir, "secrets")
	if err := os.MkdirAll(secrets, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	modePath := filepath.Join(secrets, "regatta-mode.json")
	if err := os.WriteFile(modePath, []byte("{}"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	hz := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer hz.Close()

	fx := &fakeExec{script: []fakeExecResp{
		{stdout: []byte("ok")}, {stdout: []byte("ok")}, {stdout: []byte("ok")},
	}}
	p := &RegattaProvider{
		Exec:         fx,
		HealthzURL:   hz.URL + "/healthz",
		HealthzPoll:  5 * time.Millisecond,
		HealthBudget: 500 * time.Millisecond,
	}
	if err := p.Connect(context.Background(), regattaFakeAttestor{allow: true}); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	st, err := os.Stat(modePath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("perm = %v, want 0600 (Chmod did not re-assert against hostile umask)", st.Mode().Perm())
	}
}

// TestRegattaProvider_NameAndTokenPath asserts the Provider-interface surface
// so DefaultRegistry shows regatta in `leah connect --list`.
func TestRegattaProvider_NameAndTokenPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	p := NewRegatta()
	if p.Name() != "regatta" {
		t.Fatalf("name = %q", p.Name())
	}
	want := filepath.Join(dir, "secrets", "regatta-mode.json")
	if p.TokenPath() != want {
		t.Fatalf("token path = %q, want %q", p.TokenPath(), want)
	}
}

// TestRegattaRevoke_StopsAndRemovesContainer: Revoke is the hook the generic
// Disconnect calls — it must `docker stop` then `docker rm` the pinned
// container, in that argv order.
func TestRegattaRevoke_StopsAndRemovesContainer(t *testing.T) {
	fx := &fakeExec{script: []fakeExecResp{{}, {}}}
	p := &RegattaProvider{Exec: fx}

	if err := p.Revoke(context.Background()); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if got := fx.argvAt(0); len(got) != 3 || got[0] != "docker" || got[1] != "stop" || got[2] != regattaContainer {
		t.Fatalf("call 0 not `docker stop leah-regatta`: %v", got)
	}
	if got := fx.argvAt(1); len(got) != 3 || got[0] != "docker" || got[1] != "rm" || got[2] != regattaContainer {
		t.Fatalf("call 1 not `docker rm leah-regatta`: %v", got)
	}
}

// TestRegattaRevoke_AlreadyGone_Idempotent proves a stopped/removed container
// (docker errors "No such container") yields a clean exit, not an error.
func TestRegattaRevoke_AlreadyGone_Idempotent(t *testing.T) {
	fx := &fakeExec{script: []fakeExecResp{
		{err: errors.New("Error response from daemon: No such container: leah-regatta")},
		{err: errors.New("Error: No such container: leah-regatta")},
	}}
	p := &RegattaProvider{Exec: fx}

	if err := p.Revoke(context.Background()); err != nil {
		t.Fatalf("Revoke not idempotent: %v", err)
	}
}

// TestDisconnectRegatta_TearsDownAndRemovesMode wires the regatta provider
// through the generic Disconnect: container teardown (Revoker hook) + mode-file
// removal, no parallel disconnect path.
func TestDisconnectRegatta_TearsDownAndRemovesMode(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	modePath := regattaModePath()
	if err := os.MkdirAll(filepath.Dir(modePath), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(modePath, []byte(`{"mode":"docker"}`), 0o600); err != nil {
		t.Fatalf("seed mode: %v", err)
	}
	fx := &fakeExec{script: []fakeExecResp{{}, {}}}
	p := &RegattaProvider{Exec: fx}

	if err := Disconnect(context.Background(), p, regattaFakeAttestor{allow: true}); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	if got := fx.argvAt(0); len(got) != 3 || got[1] != "stop" {
		t.Fatalf("missing docker stop: %v", got)
	}
	if got := fx.argvAt(1); len(got) != 3 || got[1] != "rm" {
		t.Fatalf("missing docker rm: %v", got)
	}
	if _, err := os.Stat(modePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("mode file still present: %v", err)
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if strings.Contains(s, want) {
			return true
		}
	}
	return false
}
