// Package osevent exposes macOS push notifications (NSWorkspace, Contacts) as
// typed Go channels. Single dedicated CFRunLoop pump on darwin; no-op stub
// elsewhere. Consumed by O3/O4/O5 in wave-10 — no duplicate cgo bridges.
package osevent

import (
	"context"
	"time"
)

// Kind identifies a push-notification class. Stable wire-shape — downstream
// consumers switch on these.
type Kind string

// KnownKinds — append-only enum.
const (
	WorkspaceActiveAppChanged Kind = "workspace.active_app_changed"
	WorkspaceLaunched         Kind = "workspace.launched"
	WorkspaceTerminated       Kind = "workspace.terminated"
	WorkspaceWillSleep        Kind = "workspace.will_sleep"
	WorkspaceDidWake          Kind = "workspace.did_wake"
	ContactStoreChanged       Kind = "contact_store_changed"
	SafariHistoryChanged      Kind = "safari.history_changed"
	FocusStateChanged         Kind = "focus.state_changed"
)

// Event is one push notification. Detail carries source-specific fields:
// active-app events use {"bundle_id": "...", "name": "..."}; sleep/wake/
// contact events leave Detail empty.
type Event struct {
	Kind   Kind
	At     time.Time
	Detail map[string]any
}

// Source delivers Events as they arrive. Close is idempotent; channels
// returned by Subscribe close on Close() or ctx.Done().
type Source interface {
	Subscribe(ctx context.Context, kind Kind) (<-chan Event, error)
	Close() error
}

// Config controls Source construction. Blocklist matches bundle-ID prefix
// (e.g. "com.bank.") at the source for operator-trust: a blocklisted
// foreground app suppresses the active-app event before any subscriber sees
// it, so a single bypass cannot leak across consumers. Blocklist applies to
// WorkspaceActiveAppChanged only; launch/terminate/sleep/wake/contact events
// are not gated.
type Config struct {
	Blocklist []string
}
