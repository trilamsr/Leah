package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/connect"
)

// runConnectRegatta is distinct from runConnect: regatta is a Docker container,
// not a token-bearing service. p=nil wires production defaults; tests inject
// a fake-Exec-backed provider.
func runConnectRegatta(ctx context.Context, args []string, w io.Writer, p *connect.RegattaProvider) int {
	if shouldShowHelp(args) {
		_, _ = fmt.Fprintln(w, "usage: leah connect regatta")
		_, _ = fmt.Fprintln(w, "")
		_, _ = fmt.Fprintln(w, "Start a local Docker container running regatta. Requires a healthy Docker daemon.")
		return 0
	}
	if p == nil {
		p = connect.NewRegatta()
		p.Exec = osExecRunner{}
	}

	a := &audit.Logger{Path: filepath.Join(stateDir(), "audit.jsonl"), DefaultWorkspace: activeWorkspace}
	p.Audit = &regattaAuditAdapter{logger: a}

	att := newConnectAttestor()
	if err := p.Connect(ctx, att); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah connect regatta: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(w, "ok: regatta running at http://127.0.0.1:9090 (container: leah-regatta)\n")
	return 0
}

// regattaAuditAdapter keeps internal/connect free of an internal/audit import.
type regattaAuditAdapter struct {
	logger *audit.Logger
}

func (r *regattaAuditAdapter) Record(e connect.AuditEntry) {
	outcome := "failed"
	if e.Success {
		outcome = "success"
	}
	detail := e.Reason
	if e.Success && e.ImageDigest != "" {
		detail = e.ImageDigest
	}
	_ = r.logger.Append(audit.Entry{
		Kind:        e.Kind,
		BlastRadius: 2,
		Outcome:     outcome,
		Detail:      detail,
	})
}

type osExecRunner struct{}

func (osExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr []byte
	out, err := cmd.Output()
	stdout = out
	if ee := new(exec.ExitError); errors.As(err, &ee) {
		stderr = ee.Stderr
	}
	return stdout, stderr, err
}
