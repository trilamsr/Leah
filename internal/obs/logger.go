// Package obs is Leah's internal observability layer — structured logs,
// in-process metrics, and panic-recovering goroutine wrappers. Without it,
// when leah-daemon hangs at 3am Leah cannot draft a regatta self-bug
// issue ("here's the panic + last 10 log lines"); the audit log captures
// user-facing actions, obs captures Leah's internal experience.
package telemetry

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// traceKey is the context key for trace IDs. Unexported so callers go
// through WithTrace + TraceID.
type ctxKey int

const (
	traceKey ctxKey = iota
	loggerKey
)

// WithTrace returns a child context carrying the given trace ID.
func WithTrace(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, traceKey, id)
}

// TraceID returns the trace ID stored in ctx, or "" if absent.
func TraceID(ctx context.Context) string {
	if v, ok := ctx.Value(traceKey).(string); ok {
		return v
	}
	return ""
}

// WithLogger returns a child context carrying the given logger.
func WithLogger(ctx context.Context, lg *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, lg)
}

// LoggerFromCtx returns the ctx-bound logger pre-populated with trace_id,
// or the global default if none is bound. Never returns nil.
func LoggerFromCtx(ctx context.Context) *slog.Logger {
	lg, _ := ctx.Value(loggerKey).(*slog.Logger)
	if lg == nil {
		lg = slog.Default()
	}
	if id := TraceID(ctx); id != "" {
		return lg.With("trace_id", id)
	}
	return lg
}

// NewLogger builds Leah's default logger: JSONL to $LEAH_STATE_DIR/logs/
// (daily-rotated) AND stderr. Level honors LEAH_LOG_LEVEL (DEBUG, INFO,
// WARN, ERROR — default INFO). Returns the logger plus a close func the
// caller MUST invoke at shutdown to release the file handle.
func NewLogger() (*slog.Logger, func(), error) {
	level := parseLevel(os.Getenv("LEAH_LOG_LEVEL"))
	stateDir := os.Getenv("LEAH_STATE_DIR")
	if stateDir == "" {
		home, _ := os.UserHomeDir()
		stateDir = filepath.Join(home, ".leah-state")
	}
	logsDir := filepath.Join(stateDir, "logs")
	if err := os.MkdirAll(logsDir, 0o700); err != nil {
		return nil, func() {}, fmt.Errorf("obs: mkdir logs: %w", err)
	}
	rot := newDailyRotator(logsDir)
	mw := &multiWriter{rotator: rot, extra: os.Stderr}

	handler := slog.NewJSONHandler(mw, &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			// Rename slog defaults to match Leah's log schema (spec §3.1):
			// time→ts so future selflearn parsers + grep recipes can rely
			// on a single field name across the codebase.
			if a.Key == slog.TimeKey {
				a.Key = "ts"
			}
			return a
		},
	})
	lg := slog.New(handler)
	closeFn := func() { rot.close() }
	return lg, closeFn, nil
}

// NewLoggerWithRing is NewLogger plus an ErrorRing that captures every
// ERROR-level log entry so diag.state can surface the last failure.
func NewLoggerWithRing(ring *ErrorRing) (*slog.Logger, func(), error) {
	level := parseLevel(os.Getenv("LEAH_LOG_LEVEL"))
	stateDir := os.Getenv("LEAH_STATE_DIR")
	if stateDir == "" {
		home, _ := os.UserHomeDir()
		stateDir = filepath.Join(home, ".leah-state")
	}
	logsDir := filepath.Join(stateDir, "logs")
	if err := os.MkdirAll(logsDir, 0o700); err != nil {
		return nil, func() {}, fmt.Errorf("obs: mkdir logs: %w", err)
	}
	rot := newDailyRotator(logsDir)
	mw := &multiWriter{rotator: rot, extra: os.Stderr}

	inner := slog.NewJSONHandler(mw, &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				a.Key = "ts"
			}
			return a
		},
	})
	lg := slog.New(&ringHandler{Handler: inner, ring: ring})
	return lg, func() { rot.close() }, nil
}

// ringHandler wraps an slog.Handler and feeds ERROR-level messages into ring.
type ringHandler struct {
	slog.Handler
	ring *ErrorRing
}

func (h *ringHandler) Handle(ctx context.Context, r slog.Record) error {
	if r.Level >= slog.LevelError && h.ring != nil {
		h.ring.Add(r.Message)
	}
	return h.Handler.Handle(ctx, r)
}

// parseLevel maps a LEAH_LOG_LEVEL string to slog.Level. Unknown → INFO.
func parseLevel(s string) slog.Level {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "DEBUG":
		return slog.LevelDebug
	case "WARN", "WARNING":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// multiWriter fans Write calls into the daily rotator AND a passthrough
// (typically stderr). Write errors on the rotator do NOT block the
// passthrough — we'd rather see a log on stderr than nothing.
type multiWriter struct {
	rotator *dailyRotator
	extra   io.Writer
}

func (m *multiWriter) Write(p []byte) (int, error) {
	_, _ = m.rotator.write(time.Now().UTC(), p)
	if m.extra != nil {
		_, _ = m.extra.Write(p)
	}
	return len(p), nil
}

// dailyRotator opens one file per UTC date under dir and atomically swaps
// + closes the prior handle on date change. The write path holds mu across
// the file.Write so rotate-close cannot race the handle out from under it.
type dailyRotator struct {
	dir string

	mu      sync.Mutex
	curDate string
	curFile *os.File
}

func newDailyRotator(dir string) *dailyRotator {
	return &dailyRotator{dir: dir}
}

// write appends p to the file for t's UTC date, rotating if needed, all
// under mu so a concurrent close() cannot reach the handle mid-write.
func (r *dailyRotator) write(t time.Time, p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	f, err := r.fileForLocked(t)
	if err != nil {
		return 0, err
	}
	return f.Write(p)
}

// writerFor returns the file handle for the given timestamp's UTC date.
// Retained for tests that observe rotation directly; production writes go
// through write() to keep the handle live across the actual Write call.
func (r *dailyRotator) writerFor(t time.Time) (io.Writer, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.fileForLocked(t)
}

// fileForLocked must be called with r.mu held. On date change the prior
// handle is closed BEFORE the new one is returned (spec §9 HIGH).
func (r *dailyRotator) fileForLocked(t time.Time) (*os.File, error) {
	date := t.Format("2006-01-02")
	if r.curFile != nil && r.curDate == date {
		return r.curFile, nil
	}
	if err := os.MkdirAll(r.dir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(r.dir, "leah-"+date+".jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	if r.curFile != nil {
		_ = r.curFile.Close()
	}
	r.curFile = f
	r.curDate = date
	return f, nil
}

func (r *dailyRotator) close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.curFile != nil {
		_ = r.curFile.Close()
		r.curFile = nil
	}
}
