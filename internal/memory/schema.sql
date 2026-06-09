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
INSERT OR IGNORE INTO schema_meta(key, value) VALUES('version', '1');

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

UPDATE schema_meta SET value='2' WHERE key='version';
