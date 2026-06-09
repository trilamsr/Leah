// Package persona owns per-workspace tone / signature / voice settings.
//
// Memory schema v5 (workspace_persona) keys one row per workspace; an
// unconfigured workspace falls back to package-level Defaults so Load
// always returns a usable Persona without forcing callers to nil-check.
//
// See docs/specs/2026-06-09-context-manager.md §6 (deferred until now —
// activated for the workspace-dimension rollout) and the overview
// docs/specs/2026-06-09-leah-overview.md §3.5 bullet 4 (per-workspace
// persona keyed on tone / signature / voice).
package persona

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// DefaultTone, DefaultSignature, DefaultVoiceID are the fallback persona
// fields when a workspace has no explicit row. Kept as package vars so
// tests + ops scripts can introspect them without re-typing the literals.
var (
	DefaultTone      = "neutral, concise, operator-facing"
	DefaultSignature = ""
	DefaultVoiceID   = ""
)

// ErrEmptyWorkspace is returned by Set when the Workspace field is empty.
var ErrEmptyWorkspace = errors.New("persona: workspace required")

// Persona is the tunable per-workspace identity Leah carries into LLM
// system prompts + downstream TTS calls.
type Persona struct {
	Workspace string
	Tone      string
	Signature string
	VoiceID   string
	// IsDefault is true when Load fell back to package defaults because no
	// explicit row exists for the workspace. Helps callers avoid prepending
	// a noisy "Persona: neutral" preamble for unconfigured workspaces.
	IsDefault bool
	UpdatedAt time.Time
}

// SystemPromptPrefix returns the one-paragraph preamble Reasoner.Ask
// prepends to its SystemPrompt for this workspace. Default personas
// produce an empty string — see TestSystemPromptPrefixSkipsDefaultPersona.
func (p Persona) SystemPromptPrefix() string {
	if p.IsDefault {
		return ""
	}
	var b strings.Builder
	b.WriteString("Workspace: ")
	b.WriteString(p.Workspace)
	b.WriteString(". ")
	if p.Tone != "" {
		b.WriteString("Tone: ")
		b.WriteString(p.Tone)
		b.WriteString(". ")
	}
	if p.Signature != "" {
		b.WriteString("Signature: ")
		b.WriteString(p.Signature)
		b.WriteString(".")
	}
	return strings.TrimSpace(b.String())
}

// Store is the persona table handle. Wraps *sql.DB; safe for concurrent use
// (SQLite WAL + busy_timeout handle the serialization).
type Store struct {
	db  *sql.DB
	now func() time.Time
}

// Open returns a Store over the SQLite file at path. Schema is applied
// idempotently — re-opens are safe.
func Open(path string) (*Store, error) {
	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	s := &Store{db: db, now: func() time.Time { return time.Now().UTC() }}
	if err := s.ensureSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close releases the underlying handle.
func (s *Store) Close() error { return s.db.Close() }

// DB exposes the underlying handle for callers (e.g. tests) that need to
// share the migrated connection.
func (s *Store) DB() *sql.DB { return s.db }

func (s *Store) ensureSchema() error {
	const ddl = `CREATE TABLE IF NOT EXISTS workspace_persona (
		workspace   TEXT PRIMARY KEY,
		tone        TEXT NOT NULL DEFAULT '',
		signature   TEXT NOT NULL DEFAULT '',
		voice_id    TEXT NOT NULL DEFAULT '',
		updated_at  TEXT NOT NULL
	)`
	if _, err := s.db.Exec(ddl); err != nil {
		return fmt.Errorf("ensure workspace_persona: %w", err)
	}
	return nil
}

// Load returns the persona for workspace, or a default-marked Persona when
// no explicit row exists (or workspace is empty). Never errors with "no row".
func (s *Store) Load(workspace string) (Persona, error) {
	if workspace == "" {
		return defaultPersona(workspace), nil
	}
	var p Persona
	var updatedAt string
	row := s.db.QueryRow(
		`SELECT workspace, tone, signature, voice_id, updated_at
		 FROM workspace_persona WHERE workspace = ?`, workspace,
	)
	err := row.Scan(&p.Workspace, &p.Tone, &p.Signature, &p.VoiceID, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return defaultPersona(workspace), nil
	}
	if err != nil {
		return Persona{}, fmt.Errorf("load persona: %w", err)
	}
	if t, perr := time.Parse(time.RFC3339, updatedAt); perr == nil {
		p.UpdatedAt = t
	}
	return p, nil
}

// Set upserts the persona row for p.Workspace.
func (s *Store) Set(p Persona) error {
	if strings.TrimSpace(p.Workspace) == "" {
		return ErrEmptyWorkspace
	}
	ts := s.now().Format(time.RFC3339)
	_, err := s.db.Exec(
		`INSERT INTO workspace_persona(workspace, tone, signature, voice_id, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(workspace) DO UPDATE SET
		   tone       = excluded.tone,
		   signature  = excluded.signature,
		   voice_id   = excluded.voice_id,
		   updated_at = excluded.updated_at`,
		p.Workspace, p.Tone, p.Signature, p.VoiceID, ts,
	)
	if err != nil {
		return fmt.Errorf("upsert persona: %w", err)
	}
	return nil
}

// SetNow overrides the clock (test-only).
func (s *Store) SetNow(fn func() time.Time) { s.now = fn }

// defaultPersona builds the fallback Persona for an unconfigured workspace.
func defaultPersona(workspace string) Persona {
	return Persona{
		Workspace: workspace,
		Tone:      DefaultTone,
		Signature: DefaultSignature,
		VoiceID:   DefaultVoiceID,
		IsDefault: true,
	}
}
