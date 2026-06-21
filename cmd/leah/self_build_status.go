package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/selfbuildstatus"
)

// runSelfBuildStatus reads the audit log, classifies each self-build into a
// closed-loop receipt, and prints one line per loop. Exit 0 when at least one
// loop is Closed (so CI / audit-session can gate on "loop runnable"); pending
// is a normal state, not a failure — only a read error is non-zero.
func runSelfBuildStatus(args []string, out io.Writer) int {
	if shouldShowHelp(args) {
		_, _ = fmt.Fprintln(os.Stderr, "usage: leah self-build-status")
		return 0
	}
	auditPath := filepath.Join(stateDir(), "audit.jsonl")
	loops, err := classifyLoops(auditPath)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah self-build-status: %v\n", err)
		return 1
	}
	if len(loops) == 0 {
		_, _ = fmt.Fprintln(out, "no self-build loops in audit log")
		return 0
	}
	for _, l := range loops {
		_, _ = fmt.Fprintln(out, formatLoop(l))
	}
	return 0
}

func classifyLoops(path string) ([]selfbuildstatus.Loop, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var entries []audit.Entry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var e audit.Entry
		if err := json.Unmarshal(line, &e); err != nil {
			return nil, fmt.Errorf("malformed audit line: %w", err)
		}
		entries = append(entries, e)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return selfbuildstatus.Classify(entries), nil
}

func formatLoop(l selfbuildstatus.Loop) string {
	s := l.ArgsHash + " "
	for _, c := range l.Checkpoints {
		x := " "
		if c.Done {
			x = "x"
		}
		s += fmt.Sprintf("[%s %s]", x, c.Name)
	}
	if l.Closed {
		return s + " CLOSED"
	}
	return s + " PENDING"
}
