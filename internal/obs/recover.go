package obs

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// SafeGo spawns f in a goroutine that recovers panics, logs them,
// increments the leah_panic_total{name=…} counter on reg, and writes the
// full stack trace to $LEAH_STATE_DIR/panics/. When the env var
// LEAH_OBS_PANIC_PROPAGATE=1 is set the inner goroutine re-panics after
// capture so test runs fail loudly.
//
// Use this wherever existing code currently bare-spawns a goroutine; the
// daemon weekly tick + dispatcher.Ship.watch are the first targets.
func SafeGo(lg *slog.Logger, reg *Registry, name string, f func()) {
	go SafeRun(lg, reg, name, f)
}

// SafeRun is the synchronous core of SafeGo — runs f on the current
// goroutine, captures panics into the log + panic file + counter, and
// re-panics when LEAH_OBS_PANIC_PROPAGATE=1. Exposed so tests can
// exercise the recover path without dispatching a goroutine the test
// runtime cannot recover from.
func SafeRun(lg *slog.Logger, reg *Registry, name string, f func()) {
	if lg == nil {
		lg = slog.Default()
	}
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		stack := captureStack()
		lg.Error("obs.panic",
			"name", name,
			"panic", fmt.Sprintf("%v", r),
			"stack", stack,
		)
		if reg != nil {
			reg.Counter("leah_panic_total").Inc(map[string]string{"name": name})
		}
		writePanicFile(name, r, stack)
		if os.Getenv("LEAH_OBS_PANIC_PROPAGATE") == "1" {
			panic(r)
		}
	}()
	f()
}

// captureStack returns a runtime stack dump for all goroutines, bounded
// at 64KB to keep panic files grep-able.
func captureStack() string {
	buf := make([]byte, 64*1024)
	n := runtime.Stack(buf, true)
	return string(buf[:n])
}

// writePanicFile drops a per-panic .txt under $LEAH_STATE_DIR/panics/.
// Best-effort: write errors are swallowed (we just panicked — disk
// failure on top is not fixable here). selflearn (Wave 3) reads these
// files when drafting Leah self-bug regatta issues.
func writePanicFile(name string, panicVal any, stack string) {
	stateDir := os.Getenv("LEAH_STATE_DIR")
	if stateDir == "" {
		home, _ := os.UserHomeDir()
		stateDir = filepath.Join(home, ".leah-state")
	}
	dir := filepath.Join(stateDir, "panics")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	now := time.Now().UTC()
	fname := fmt.Sprintf("%s-%s.txt", now.Format("2006-01-02T15-04-05.000"), sanitize(name))
	path := filepath.Join(dir, fname)
	body := fmt.Sprintf("panic: %v\ngoroutine: %s\nts: %s\n\n%s",
		panicVal, name, now.Format(time.RFC3339), stack)
	_ = os.WriteFile(path, []byte(body), 0o600)
}

// sanitize strips path separators + spaces from a goroutine name so the
// filename stays single-segment + shell-friendly.
func sanitize(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch r {
		case '/', '\\', ' ', '\t', '\n':
			out = append(out, '-')
		default:
			out = append(out, r)
		}
	}
	return string(out)
}
