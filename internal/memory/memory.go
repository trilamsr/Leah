// Package memory is the single-operator persistent KB for leah (M2).
// Three nouns: contact, project, decision. Storage only — no retrieval / no Reasoner integration yet.
// See docs/specs/2026-06-09-memory-m2-minimal.md.
package memory

import (
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

const embeddedSchemaVersion = "6"

// schemaMetaBootstrapSQL creates the version-tracking table only.
// Kept separate from schemaSQL so we can read the on-disk version BEFORE
// applying the full DDL — otherwise a stamp-on-every-open inside schema.sql
// would clobber an operator-bumped value before the newer-than-binary guard
// fires (Wave3-P regression).
const schemaMetaBootstrapSQL = `CREATE TABLE IF NOT EXISTS schema_meta (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);`

// Store is the memory KB handle. Wrap *sql.DB; one per process.
type Store struct {
	db      *sql.DB
	now     func() time.Time
	OnWrite func(table, id string) // optional audit hook, fired post-commit
}

// Contact is a person tri tracks.
type Contact struct {
	ID          string
	WorkspaceID string
	Name        string
	Email       string
	Notes       string
	CreatedAt   string
	UpdatedAt   string
}

// Project is a workstream tri tracks.
type Project struct {
	ID          string
	WorkspaceID string
	Name        string
	Status      string // active|paused|done
	Notes       string
	CreatedAt   string
	UpdatedAt   string
}

// Decision is a recorded choice + rationale.
type Decision struct {
	ID          string
	WorkspaceID string
	Topic       string
	Choice      string
	Rationale   string
	DecidedAt   string // operator-supplied; may backdate
	CreatedAt   string // recorded-in-DB time
}

// NewStore opens (creating if needed) the SQLite file at path and runs schema migration.
// Idempotent: safe to call multiple times against the same path.
func NewStore(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("mkdir state dir: %w", err)
	}
	// WAL = better concurrency between leah CLI and leah-daemon both holding the DB.
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", url.PathEscape(path))
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// Single writer keeps modernc + WAL out of contention storms; reads still parallel via WAL.
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	s := &Store{db: db, now: func() time.Time { return time.Now().UTC() }}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close releases the underlying DB handle.
func (s *Store) Close() error { return s.db.Close() }

// DB returns the underlying *sql.DB. Exposed so sibling packages (e.g.
// selflearn.MistakeStore) can share the single migrated handle rather than
// opening their own connection + re-running schema.
func (s *Store) DB() *sql.DB { return s.db }

func (s *Store) migrate() error {
	// Bootstrap schema_meta first so we can read the on-disk version BEFORE
	// applying the full DDL. Order matters: reading after exec(schemaSQL)
	// would be racing a stamp that no longer exists, but ordering also
	// guarantees we never run DDL against a DB whose schema is newer than
	// this binary understands.
	if _, err := s.db.Exec(schemaMetaBootstrapSQL); err != nil {
		return fmt.Errorf("bootstrap schema_meta: %w", err)
	}
	var v string
	err := s.db.QueryRow(`SELECT value FROM schema_meta WHERE key='version'`).Scan(&v)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read schema version: %w", err)
	}
	// Lex compare ranks "10" < "9"; parse to int so v10+ orders correctly (#24).
	// On parse failure surface the corruption — silent lex fallback would mask it.
	if v != "" {
		onDisk, perr := parseSchemaVersion(v)
		if perr != nil {
			return fmt.Errorf("parse on-disk schema version %q: %w", v, perr)
		}
		embedded, perr := parseSchemaVersion(embeddedSchemaVersion)
		if perr != nil {
			return fmt.Errorf("parse embedded schema version %q: %w", embeddedSchemaVersion, perr)
		}
		if onDisk > embedded {
			return fmt.Errorf("memory.db schema version %s newer than binary %s; upgrade leah", v, embeddedSchemaVersion)
		}
	}
	// Safe to apply DDL: on-disk version is absent or ≤ embedded.
	if _, err := s.db.Exec(schemaSQL); err != nil {
		return fmt.Errorf("exec schema: %w", err)
	}
	// Stamp to embedded version. INSERT OR REPLACE is safe now that we've
	// already proven on-disk ≤ embedded (no downgrade-overwrite risk).
	if _, err := s.db.Exec(
		`INSERT OR REPLACE INTO schema_meta(key, value) VALUES('version', ?)`,
		embeddedSchemaVersion,
	); err != nil {
		return fmt.Errorf("stamp schema version: %w", err)
	}
	return nil
}

func newULID() string {
	return ulid.Make().String()
}

func parseSchemaVersion(s string) (int, error) {
	return strconv.Atoi(strings.TrimSpace(s))
}

func (s *Store) fireWrite(table, id string) {
	if s.OnWrite != nil {
		s.OnWrite(table, id)
	}
}

// --- Contact ---

// AddContact inserts a Contact; populates ID/CreatedAt/UpdatedAt/WorkspaceID.
func (s *Store) AddContact(c Contact) (Contact, error) {
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" {
		return Contact{}, errors.New("contact: name required")
	}
	if c.WorkspaceID == "" {
		c.WorkspaceID = "default"
	}
	c.ID = newULID()
	ts := s.now().Format(time.RFC3339)
	c.CreatedAt, c.UpdatedAt = ts, ts
	_, err := s.db.Exec(
		`INSERT INTO contact(id, workspace_id, name, email, notes, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.WorkspaceID, c.Name, nullable(c.Email), nullable(c.Notes), c.CreatedAt, c.UpdatedAt,
	)
	if err != nil {
		return Contact{}, fmt.Errorf("insert contact: %w", err)
	}
	s.fireWrite("contact", c.ID)
	return c, nil
}

// ListContacts returns contacts in the default workspace, name-sorted.
// Backward-compat wrapper for ListContactsByWorkspace("default").
func (s *Store) ListContacts() ([]Contact, error) {
	return s.ListContactsByWorkspace("default")
}

// ListContactsByWorkspace returns contacts tagged with the supplied workspace,
// name-sorted. Empty workspace is canonicalized to "default" so callers may
// pass the operator's active workspace without checking for empty.
func (s *Store) ListContactsByWorkspace(workspace string) ([]Contact, error) {
	if workspace == "" {
		workspace = "default"
	}
	rows, err := s.db.Query(
		`SELECT id, workspace_id, name, COALESCE(email,''), COALESCE(notes,''), created_at, updated_at
		 FROM contact WHERE workspace_id=? ORDER BY name`, workspace,
	)
	if err != nil {
		return nil, fmt.Errorf("query contacts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []Contact
	for rows.Next() {
		var c Contact
		if err := rows.Scan(&c.ID, &c.WorkspaceID, &c.Name, &c.Email, &c.Notes, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan contact: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetContact fetches a single contact by ID.
func (s *Store) GetContact(id string) (Contact, error) {
	var c Contact
	err := s.db.QueryRow(
		`SELECT id, workspace_id, name, COALESCE(email,''), COALESCE(notes,''), created_at, updated_at
		 FROM contact WHERE id=?`, id,
	).Scan(&c.ID, &c.WorkspaceID, &c.Name, &c.Email, &c.Notes, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return Contact{}, fmt.Errorf("get contact %s: %w", id, err)
	}
	return c, nil
}

// --- Project ---

// AddProject inserts a Project; populates ID/timestamps/defaults.
func (s *Store) AddProject(p Project) (Project, error) {
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		return Project{}, errors.New("project: name required")
	}
	if p.Status == "" {
		p.Status = "active"
	}
	if p.WorkspaceID == "" {
		p.WorkspaceID = "default"
	}
	p.ID = newULID()
	ts := s.now().Format(time.RFC3339)
	p.CreatedAt, p.UpdatedAt = ts, ts
	_, err := s.db.Exec(
		`INSERT INTO project(id, workspace_id, name, status, notes, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.WorkspaceID, p.Name, p.Status, nullable(p.Notes), p.CreatedAt, p.UpdatedAt,
	)
	if err != nil {
		return Project{}, fmt.Errorf("insert project: %w", err)
	}
	s.fireWrite("project", p.ID)
	return p, nil
}

// ListProjects returns projects in the default workspace, name-sorted.
// Backward-compat wrapper for ListProjectsByWorkspace("default").
func (s *Store) ListProjects() ([]Project, error) {
	return s.ListProjectsByWorkspace("default")
}

// ListProjectsByWorkspace returns projects for the supplied workspace,
// name-sorted. Empty workspace canonicalizes to "default".
func (s *Store) ListProjectsByWorkspace(workspace string) ([]Project, error) {
	if workspace == "" {
		workspace = "default"
	}
	rows, err := s.db.Query(
		`SELECT id, workspace_id, name, status, COALESCE(notes,''), created_at, updated_at
		 FROM project WHERE workspace_id=? ORDER BY name`, workspace,
	)
	if err != nil {
		return nil, fmt.Errorf("query projects: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.WorkspaceID, &p.Name, &p.Status, &p.Notes, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetProject fetches a single project by ID.
func (s *Store) GetProject(id string) (Project, error) {
	var p Project
	err := s.db.QueryRow(
		`SELECT id, workspace_id, name, status, COALESCE(notes,''), created_at, updated_at
		 FROM project WHERE id=?`, id,
	).Scan(&p.ID, &p.WorkspaceID, &p.Name, &p.Status, &p.Notes, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return Project{}, fmt.Errorf("get project %s: %w", id, err)
	}
	return p, nil
}

// --- Decision ---

// AddDecision inserts a Decision; populates ID/created_at/defaults.
func (s *Store) AddDecision(d Decision) (Decision, error) {
	d.Topic = strings.TrimSpace(d.Topic)
	d.Choice = strings.TrimSpace(d.Choice)
	if d.Topic == "" {
		return Decision{}, errors.New("decision: topic required")
	}
	if d.Choice == "" {
		return Decision{}, errors.New("decision: choice required")
	}
	if d.WorkspaceID == "" {
		d.WorkspaceID = "default"
	}
	d.ID = newULID()
	d.CreatedAt = s.now().Format(time.RFC3339)
	if d.DecidedAt == "" {
		d.DecidedAt = d.CreatedAt
	}
	_, err := s.db.Exec(
		`INSERT INTO decision(id, workspace_id, topic, choice, rationale, decided_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		d.ID, d.WorkspaceID, d.Topic, d.Choice, nullable(d.Rationale), d.DecidedAt, d.CreatedAt,
	)
	if err != nil {
		return Decision{}, fmt.Errorf("insert decision: %w", err)
	}
	s.fireWrite("decision", d.ID)
	return d, nil
}

// ListDecisions returns decisions in the default workspace, newest decided first.
// Backward-compat wrapper for ListDecisionsByWorkspace("default").
func (s *Store) ListDecisions() ([]Decision, error) {
	return s.ListDecisionsByWorkspace("default")
}

// ListDecisionsByWorkspace returns decisions for the supplied workspace,
// newest decided first. Empty workspace canonicalizes to "default".
func (s *Store) ListDecisionsByWorkspace(workspace string) ([]Decision, error) {
	if workspace == "" {
		workspace = "default"
	}
	rows, err := s.db.Query(
		`SELECT id, workspace_id, topic, choice, COALESCE(rationale,''), decided_at, created_at
		 FROM decision WHERE workspace_id=? ORDER BY decided_at DESC`, workspace,
	)
	if err != nil {
		return nil, fmt.Errorf("query decisions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []Decision
	for rows.Next() {
		var d Decision
		if err := rows.Scan(&d.ID, &d.WorkspaceID, &d.Topic, &d.Choice, &d.Rationale, &d.DecidedAt, &d.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan decision: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// GetDecision fetches a single decision by ID.
func (s *Store) GetDecision(id string) (Decision, error) {
	var d Decision
	err := s.db.QueryRow(
		`SELECT id, workspace_id, topic, choice, COALESCE(rationale,''), decided_at, created_at
		 FROM decision WHERE id=?`, id,
	).Scan(&d.ID, &d.WorkspaceID, &d.Topic, &d.Choice, &d.Rationale, &d.DecidedAt, &d.CreatedAt)
	if err != nil {
		return Decision{}, fmt.Errorf("get decision %s: %w", id, err)
	}
	return d, nil
}

// nullable turns "" into a SQL NULL so optional TEXT columns stay null vs empty-string.
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
