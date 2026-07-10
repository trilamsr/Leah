CREATE TABLE sync_peer (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL,
    paired_at     INTEGER NOT NULL,
    paused        INTEGER NOT NULL DEFAULT 0,
    last_seen_at  INTEGER,
    fingerprint   BLOB NOT NULL
);

CREATE TABLE sync_clock (
    table_name    TEXT NOT NULL,
    row_id        INTEGER NOT NULL,
    node_uuid     TEXT NOT NULL,
    lamport       INTEGER NOT NULL,
    PRIMARY KEY(table_name, row_id, node_uuid)
);

CREATE TABLE sync_tombstone (
    table_name    TEXT NOT NULL,
    row_id        INTEGER NOT NULL,
    deleted_at    INTEGER NOT NULL,
    deleted_by    TEXT NOT NULL,
    PRIMARY KEY(table_name, row_id)
);

CREATE TABLE sync_outbox (
    id            INTEGER PRIMARY KEY,
    peer_id       TEXT NOT NULL REFERENCES sync_peer(id) ON DELETE CASCADE,
    payload       BLOB NOT NULL,
    enqueued_at   INTEGER NOT NULL,
    sent_at       INTEGER
);

-- Phase 3 CRDT participants. Spec §2.5 calls for ALTER TABLE ADD COLUMN
-- node_uuid against shipped Phase 3 tables; Phase 3 never shipped these
-- four, so the additive-only outcome is "create them now with node_uuid
-- already present" — same end state, no destructive migration ever runs.
CREATE TABLE memory (
    id          INTEGER PRIMARY KEY,
    kind        TEXT NOT NULL,
    body        TEXT NOT NULL,
    created_at  INTEGER NOT NULL,
    node_uuid   TEXT NOT NULL DEFAULT 'self'
);

CREATE TABLE conversation (
    id          INTEGER PRIMARY KEY,
    started_at  INTEGER NOT NULL,
    ended_at    INTEGER,
    node_uuid   TEXT NOT NULL DEFAULT 'self'
);

CREATE TABLE pin (
    id          INTEGER PRIMARY KEY,
    widget_id   TEXT NOT NULL,
    position    INTEGER NOT NULL,
    created_at  INTEGER NOT NULL,
    node_uuid   TEXT NOT NULL DEFAULT 'self'
);

CREATE TABLE settings_kv (
    key         TEXT PRIMARY KEY,
    value       TEXT NOT NULL,
    updated_at  INTEGER NOT NULL,
    node_uuid   TEXT NOT NULL DEFAULT 'self'
);
