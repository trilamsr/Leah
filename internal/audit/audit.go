// Package audit is Leah's append-only JSONL log of operator-facing actions.
// One row per CLI invocation / daemon transition; downstream tools (status,
// retro, patterns, operatormodel) read it back. Single source of truth for
// "what did Leah do".
package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// Entry is one audit row. Stable on-disk schema — fields are read by
// patterns.Detect, selflearn.Resolver, operatormodel.Observe* and
// web.Snapshot; renaming breaks the entire downstream consumer set.
type Entry struct {
	Timestamp   string  `json:"ts"`
	Kind        string  `json:"kind"`
	ArgsHash    string  `json:"args_hash"`
	BlastRadius int     `json:"blast_radius"`
	Outcome     string  `json:"outcome"`
	CostDollars float64 `json:"cost_dollars,omitempty"`
	Detail      string  `json:"detail,omitempty"`
	// Workspace tags the row with the operator's active workspace at
	// append time. Empty/unset (legacy rows + non-workspace-aware paths)
	// are treated as the implicit "default" workspace by downstream readers.
	Workspace string `json:"workspace,omitempty"`
}

// Logger is the append-only writer for an audit.jsonl file. Concurrent
// Append calls are safe because each call re-opens the file in O_APPEND
// mode — the OS handles serialization at the syscall boundary.
type Logger struct {
	Path string
	Now  func() time.Time
	// DefaultWorkspace, if set, is consulted at Append time when the
	// incoming Entry has no Workspace tag. Lets the CLI wire active
	// workspace once at logger construction and forget about it at every
	// callsite. Caller-supplied Entry.Workspace always wins.
	DefaultWorkspace func() string
	// OnAppend fires once per Append with the error result (nil on success).
	// Wired by cmd/leah-daemon to feed obs counters. nil-safe.
	OnAppend func(error)
	// OnSubscriberDrop fires when a subscriber's bounded channel overflows
	// and an entry is dropped to make room for the newer one. Wired to the
	// leah_audit_subscriber_dropped_total{subscriber} counter. nil-safe.
	OnSubscriberDrop func(subscriber string, n int64)

	subsMu       sync.Mutex
	subs         []*subscription
	totalDropped uint64 // atomic; cumulative across lifetime, survives cancel
}

// Append writes e as a single JSON line, stamping Timestamp from Now (or
// time.Now().UTC() when unset). File is opened 0600 to keep the operator's
// audit history off other users' read paths.
func (l *Logger) Append(e Entry) (retErr error) {
	defer func() {
		if l.OnAppend != nil {
			l.OnAppend(retErr)
		}
	}()
	if l.Now != nil {
		e.Timestamp = l.Now().Format(time.RFC3339)
	} else {
		e.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	if e.Workspace == "" && l.DefaultWorkspace != nil {
		e.Workspace = l.DefaultWorkspace()
	}
	f, err := os.OpenFile(l.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open audit log: %w", err)
	}
	defer func() { _ = f.Close() }()
	buf, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("marshal entry: %w", err)
	}
	if _, err := f.Write(append(buf, '\n')); err != nil {
		return fmt.Errorf("write entry: %w", err)
	}
	l.fanout(e)
	return nil
}
