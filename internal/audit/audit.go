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

	"github.com/trilam/leah/internal/obs"
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

	// W92+W93 LLM-dim schema extension (S2 §2). All omitempty so legacy
	// rows stay byte-identical and pre-W92 readers ignore unknown keys.
	// LatencyMS / EgressBytes are int64 per spec §2 — guards against a
	// hypothetical 32-bit build target silently truncating.
	Model        string `json:"model,omitempty"`
	PromptSHA    string `json:"prompt_sha,omitempty"`
	InputTokens  int    `json:"input_tokens,omitempty"`
	OutputTokens int    `json:"output_tokens,omitempty"`
	LatencyMS    int64  `json:"latency_ms,omitempty"`
	EgressBytes  int64  `json:"egress_bytes,omitempty"`
	CacheHit     bool   `json:"cache_hit,omitempty"`
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
	// quiesceMu is held shared (RLock) by every Append and exclusive (Lock)
	// by QuiesceForConsolidation. The consolidator must own the writer side
	// while rewriting audit.jsonl on a tmp+rename path (§4 step 7) so no
	// Append can write to the orphaned inode and silently lose its row.
	quiesceMu sync.RWMutex
}

// Append writes e as a single JSON line, stamping Timestamp from Now (or
// time.Now().UTC() when unset). File is opened 0600 to keep the operator's
// audit history off other users' read paths.
func (l *Logger) Append(e Entry) (retErr error) {
	l.quiesceMu.RLock()
	defer l.quiesceMu.RUnlock()
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
	obs.Publish(obs.Event{
		TS:      time.Now().UTC(),
		Kind:    "audit.append",
		Actor:   "audit",
		Outcome: "ok",
		Target:  e.Kind,
		Detail:  e.Detail,
		Payload: e,
	})
	return nil
}

// QuiesceForConsolidation blocks new Append calls and returns a release
// handle. Callers (the W125 ConsolidatePass) hold the exclusive lock while
// rewriting audit.jsonl via tmp+rename so an Append racing the swap cannot
// land on the orphaned pre-rename inode. The returned func is idempotent —
// double-fire is a no-op rather than a double-unlock panic.
func (l *Logger) QuiesceForConsolidation() func() {
	l.quiesceMu.Lock()
	var once sync.Once
	return func() { once.Do(l.quiesceMu.Unlock) }
}
