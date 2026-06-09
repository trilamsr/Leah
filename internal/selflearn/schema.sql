-- selflearn temporary schema (will move into internal/memory/schema.sql
-- once Wave1-A lands; see docs/specs/2026-06-09-self-learning-personal.md §3.1)

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
