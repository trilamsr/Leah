package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// repoRootFromTest resolves the leah repo root from this test file's location
// so runSpecs can find scripts/audit-stale-specs.sh regardless of the `go test`
// invocation CWD.
func repoRootFromTest(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(thisFile), "..", "..")
}

func TestRunSpecs_DefaultOutputContainsSHIPPED(t *testing.T) {
	t.Setenv("LEAH_REPO_ROOT", repoRootFromTest(t))
	var buf bytes.Buffer
	code := runSpecs(nil, &buf)
	if code != 0 {
		t.Fatalf("runSpecs exit=%d, want 0; out=%q", code, buf.String())
	}
	if !strings.Contains(buf.String(), "SHIPPED\t") {
		t.Fatalf("output missing SHIPPED rows; got %q", buf.String())
	}
}

func TestRunSpecs_StaleOnlyFilters(t *testing.T) {
	t.Setenv("LEAH_REPO_ROOT", repoRootFromTest(t))
	var buf bytes.Buffer
	code := runSpecs([]string{"--stale-only"}, &buf)
	if code != 0 {
		t.Fatalf("runSpecs exit=%d, want 0; out=%q", code, buf.String())
	}
	out := strings.TrimSpace(buf.String())
	// Empty output is legitimate when the tree has zero STALE rows (the
	// healthy steady state). The falsifiability comes from rejecting any
	// non-STALE row that slipped through — if --stale-only emitted SHIPPED
	// or PARTIAL, the filter is broken. Counter-evidence that the filter
	// fires at all is covered by the unit-level test in audit-stale-specs.
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "STALE\t") {
			t.Fatalf("--stale-only leaked non-STALE row: %q", line)
		}
	}
}

func TestRunSpecs_MissingScriptReturnsNonzero(t *testing.T) {
	// Point at an empty dir with no scripts/ subdir to force the
	// missing-script branch without mutating the real tree.
	empty := t.TempDir()
	t.Setenv("LEAH_REPO_ROOT", empty)
	var buf bytes.Buffer
	code := runSpecs(nil, &buf)
	if code == 0 {
		t.Fatalf("runSpecs returned 0 with missing script; want non-zero")
	}
	if _, err := os.Stat(filepath.Join(empty, "scripts", "audit-stale-specs.sh")); err == nil {
		t.Fatalf("empty repo unexpectedly contains the script")
	}
}
