-- schema_version: 1
-- Personal-use single-operator memory: contact / project / decision.
-- workspace_id is dormant (always 'default') until multi-operator Phase X.

CREATE TABLE IF NOT EXISTS contact (
  id           TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL DEFAULT 'default',
  name         TEXT NOT NULL,
  email        TEXT,
  notes        TEXT,
  created_at   TEXT NOT NULL,
  updated_at   TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_contact_name ON contact(name);

CREATE TABLE IF NOT EXISTS project (
  id           TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL DEFAULT 'default',
  name         TEXT NOT NULL,
  status       TEXT NOT NULL DEFAULT 'active',
  notes        TEXT,
  created_at   TEXT NOT NULL,
  updated_at   TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_project_status ON project(status);

CREATE TABLE IF NOT EXISTS decision (
  id           TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL DEFAULT 'default',
  topic        TEXT NOT NULL,
  choice       TEXT NOT NULL,
  rationale    TEXT,
  decided_at   TEXT NOT NULL,
  created_at   TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_decision_topic ON decision(topic);

CREATE TABLE IF NOT EXISTS schema_meta (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
-- NOTE: schema_meta is seeded + stamped from Go (NewStore.migrate), NOT here.
-- Embedding `INSERT/UPDATE schema_meta` in this DDL file would clobber any
-- operator-bumped value before the "newer than binary" guard could fire.

-- schema_version: 2 (additive — ctxmgr tables)
-- See docs/specs/2026-06-09-context-manager.md

CREATE TABLE IF NOT EXISTS context (
  name         TEXT PRIMARY KEY,
  created_at   TEXT NOT NULL,
  description  TEXT
);

CREATE TABLE IF NOT EXISTS operator_state (
  id              INTEGER PRIMARY KEY CHECK(id=1),
  active_context  TEXT NOT NULL REFERENCES context(name),
  updated_at      TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS context_switch_log (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  from_context  TEXT,
  to_context    TEXT NOT NULL,
  switched_at   TEXT NOT NULL,
  reason        TEXT
);
CREATE INDEX IF NOT EXISTS idx_switch_time ON context_switch_log(switched_at);

INSERT OR IGNORE INTO context(name, created_at, description)
  VALUES ('default', strftime('%Y-%m-%dT%H:%M:%SZ','now'), 'Implicit context for unsegmented work');
INSERT OR IGNORE INTO operator_state(id, active_context, updated_at)
  VALUES (1, 'default', strftime('%Y-%m-%dT%H:%M:%SZ','now'));

-- schema_version: 3 (additive — selflearn mistake_log)
-- See docs/specs/2026-06-09-self-learning-personal.md §3.1

CREATE TABLE IF NOT EXISTS mistake_log (
  id          TEXT PRIMARY KEY,    -- ulid
  created_at  TIMESTAMP NOT NULL,  -- RFC3339 UTC
  audit_ts    TEXT NOT NULL,       -- audit.Entry.Timestamp (composite key part)
  audit_kind  TEXT NOT NULL,       -- audit.Entry.Kind         (composite key part)
  audit_hash  TEXT NOT NULL,       -- audit.Entry.ArgsHash     (composite key part)
  root_cause  TEXT NOT NULL,       -- short tag, e.g. "wrong-pr"
  prevention  TEXT NOT NULL        -- free-form operator note
);

CREATE INDEX IF NOT EXISTS mistake_log_created ON mistake_log(created_at);

-- schema_version: 4 (additive — operatormodel operator_profile)
-- See docs/specs/2026-06-09-operator-model.md §2

CREATE TABLE IF NOT EXISTS operator_profile (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  class        TEXT NOT NULL,    -- 'time_of_day' | 'context_transition' | 'cadence'
  key          TEXT NOT NULL,    -- per-class natural key (e.g. action kind)
  slot         TEXT NOT NULL,    -- per-class bucket (e.g. '09', 'Sun', 'leah.ask')
  count        INTEGER NOT NULL, -- raw observed events for (class, key, slot)
  weight       REAL NOT NULL,    -- decay-adjusted count; recommend() sorts on this
  window_start TEXT NOT NULL,    -- RFC3339 — bounds the audit slice that produced this row
  window_end   TEXT NOT NULL,    -- RFC3339 — typically "now" at observe time
  updated_at   TEXT NOT NULL     -- RFC3339
);
CREATE INDEX IF NOT EXISTS idx_op_profile_lookup ON operator_profile(class, key);

CREATE TABLE IF NOT EXISTS operator_profile_meta (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

-- schema_version: 5 (additive — embedding rows for semantic recall)
-- See docs/specs/2026-06-09-semantic-recall.md
-- Vector is a little-endian float32 BLOB (4*dim bytes); search is brute-force
-- cosine in Go (single-operator scale; sqlite-vec deferred per spec §Deferred).

CREATE TABLE IF NOT EXISTS embedding (
  item_id    TEXT NOT NULL,
  item_type  TEXT NOT NULL,    -- 'audit' | 'contact' | 'project' | 'decision'
  model      TEXT NOT NULL,    -- generator Name() — cross-model rows are incomparable
  dim        INTEGER NOT NULL, -- vector length; defends against model-swap drift
  vector     BLOB NOT NULL,    -- little-endian float32, length = dim*4
  content    TEXT NOT NULL,    -- the embedded text (for result display)
  updated_at TEXT NOT NULL,
  PRIMARY KEY (item_id, item_type)
);
CREATE INDEX IF NOT EXISTS idx_embedding_model ON embedding(model, dim);

-- schema_version: 6 (additive — workspace_persona)
-- See docs/specs/2026-06-09-leah-overview.md §3.5 bullet 4 + internal/persona.
-- Per-workspace persona settings consumed by reasoner SystemPrompt prefix
-- and voice TTS calls. workspace column is plain TEXT (no FK) because the
-- canonical workspace registry lives in the context table; FK at write time
-- would force persona Set to error before the operator runs `leah workspace
-- new`. Persona Load tolerates dangling workspaces (falls back to defaults).

CREATE TABLE IF NOT EXISTS workspace_persona (
  workspace   TEXT PRIMARY KEY,
  tone        TEXT NOT NULL DEFAULT '',
  signature   TEXT NOT NULL DEFAULT '',
  voice_id    TEXT NOT NULL DEFAULT '',
  updated_at  TEXT NOT NULL
);

-- schema_version: 7 (additive — W124/S9 nightly consolidation pass)
-- See docs/engineer/specs/2026-06-10-memory-consolidation.md §3
-- Summary rows collapse stable (class, key, slot) cells so
-- Profile.Update reads cheap durable summaries for >14d history.

CREATE TABLE IF NOT EXISTS operator_profile_consolidated (
  class                  TEXT NOT NULL,
  key                    TEXT NOT NULL,
  slot                   TEXT NOT NULL,
  weight                 REAL NOT NULL,
  count                  INTEGER NOT NULL,
  first_seen_ts          TEXT NOT NULL,
  last_consolidated_at   TEXT NOT NULL,
  source_window_end      TEXT NOT NULL,
  PRIMARY KEY (class, key, slot)
);
CREATE INDEX IF NOT EXISTS idx_consolidated_last
  ON operator_profile_consolidated(last_consolidated_at);

-- §3.3 — 14d-anchor for §2 rule 2 (delta gate). Each pass writes one
-- snapshot row per consolidated cell; the next pass reads w(T-14d)
-- from here rather than re-scanning raw audit rows that already aged out.
CREATE TABLE IF NOT EXISTS operator_profile_snapshot (
  class                TEXT NOT NULL,
  key                  TEXT NOT NULL,
  slot                 TEXT NOT NULL,
  weight_at_snapshot   REAL NOT NULL,
  snapshot_ts          TEXT NOT NULL,
  PRIMARY KEY (class, key, slot)
);
