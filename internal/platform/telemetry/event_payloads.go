package telemetry

import "time"

// Event is one row in the causal timeline — internal sibling to audit.jsonl;
// captures denied/failed paths audit elides (spec §2.1). Payload carries
// transport-only structured data (e.g. HUD state snapshot) on the SSE path;
// SQLite persistence ignores it so the row schema stays narrow.
type Event struct {
	TS        time.Time   `json:"ts"`
	Kind      string      `json:"kind"`
	Actor     string      `json:"actor"`
	Target    string      `json:"target,omitempty"`
	Scope     string      `json:"scope,omitempty"`
	LatencyMS int64       `json:"latency_ms,omitempty"`
	Outcome   string      `json:"outcome"`
	RefID     string      `json:"ref_id,omitempty"`
	Detail    string      `json:"detail,omitempty"`
	Payload   interface{} `json:"payload,omitempty"`
}

// HUDStateEvent is the Payload schema for `hud.state` events consumed by
// ambient.js. Field names MUST stay in sync with the JS reader
// (`p.value`, `p.listening`, `p.thinking`) — anything else freezes the
// state pill at its default.
type HUDStateEvent struct {
	Value     string `json:"value"`
	Listening bool   `json:"listening"`
	Thinking  bool   `json:"thinking"`
}

// WorkspaceActiveAppEvent is the Payload schema for
// `workspace.active_app_changed`. Separate kind from `hud.state` so the
// active-app push pump cannot reset the HUD pill or inject phantom
// recommender signals (V9 reviewer B1).
type WorkspaceActiveAppEvent struct {
	BundleID string `json:"bundle_id"`
	Name     string `json:"name"`
}

// ContactStoreChangedEvent is the Payload schema for `contact_store_changed`.
// Empty struct — the notification itself is the signal; CNContactStore does
// not name the changed records (privacy-by-design).
type ContactStoreChangedEvent struct{}

// MessagesChangedEvent is the Payload for `messages_changed`. The FSEvents
// WAL watcher never opens chat.db — consumers re-query under their own
// macos:messages:query scope, so no row data ships in the payload.
type MessagesChangedEvent struct{}

// MailChangedEvent — Envelope Index WAL mutation; consumers re-query under
// macos:mail:query.
type MailChangedEvent struct{}

// NotesChangedEvent — NoteStore.sqlite WAL mutation; consumers re-query under
// macos:notes:query.
type NotesChangedEvent struct{}

// SafariHistoryChangedEvent is the Payload schema for `safari.history_changed`.
// Empty struct — the FSEvent fires once History.db WAL has flushed; row ids
// are intentionally not surfaced (PII; downstream re-reads the db).
type SafariHistoryChangedEvent struct{}

// FocusStateChangedEvent is the Payload schema for `focus.state_changed`. Empty
// struct — pmset / DND-Assertions FSEvents only signal that focus toggled;
// consumers re-read state under macos:focus:query for the active mode.
type FocusStateChangedEvent struct{}

// CalendarStoreChangedEvent is the Payload for `calendar.store_changed`.
// Empty struct — the FSEvent fires once Calendar.sqlitedb WAL has flushed;
// event ids stay implicit so consumers re-query under macos:calendar:query.
type CalendarStoreChangedEvent struct{}

// PhotosLibraryChangedEvent — Photos.sqlite WAL mutation; consumers re-query
// under macos:photos:query. UUIDs are not surfaced (PII).
type PhotosLibraryChangedEvent struct{}

// RemindersStoreChangedEvent — Reminders Group Container store WAL mutation;
// consumers re-query under macos:reminders:query.
type RemindersStoreChangedEvent struct{}
