package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// runSpecs shells out to scripts/audit-stale-specs.sh and streams its TSV
// classification (SHIPPED / PARTIAL / STALE) to w. Flags:
//
//	--stale-only   drop SHIPPED + PARTIAL rows; emit STALE only
//
// TODO(--json): the underlying script emits TSV; v1 ships pass-through.
// Convert to structured JSON once we have a second consumer that needs it.
//
// Repo root is resolved via runtime.Caller so the binary works regardless of
// invocation CWD; LEAH_REPO_ROOT overrides for tests + sandboxed runs.
func runSpecs(args []string, w io.Writer) int {
	staleOnly := false
	for _, a := range args {
		switch a {
		case "--stale-only":
			staleOnly = true
		case "-h", "--help":
			printSpecsHelp(w)
			return 0
		default:
			_, _ = fmt.Fprintf(os.Stderr, "leah specs: unexpected argument %q\n", a)
			return 2
		}
	}

	root := specsRepoRoot()
	script := filepath.Join(root, "scripts", "audit-stale-specs.sh")
	if _, err := os.Stat(script); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah specs: missing script %s: %v\n", script, err)
		return 1
	}

	cmd := exec.Command("bash", script, "--root", root)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah specs: %v\n", err)
		return 1
	}

	if !staleOnly {
		_, _ = io.Copy(w, &out)
		return 0
	}
	sc := bufio.NewScanner(&out)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "STALE\t") {
			_, _ = fmt.Fprintln(w, line)
		}
	}
	return 0
}

// specsRepoRoot resolves the repo root. Operator override via LEAH_REPO_ROOT
// wins so a relocated binary (~/bin/leah after self-upgrade) can still find
// the script; the runtime.Caller fallback keeps `go run` + tests working.
func specsRepoRoot() string {
	if r := os.Getenv("LEAH_REPO_ROOT"); r != "" {
		return r
	}
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "."
	}
	return filepath.Join(filepath.Dir(thisFile), "..", "..")
}

func printSpecsHelp(w io.Writer) {
	_, _ = fmt.Fprintln(w, "usage: leah specs [--stale-only]")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Lists docs/engineer/specs/*.md classified as SHIPPED / PARTIAL / STALE.")
	_, _ = fmt.Fprintln(w, "Wraps scripts/audit-stale-specs.sh; output is tab-separated.")
}
