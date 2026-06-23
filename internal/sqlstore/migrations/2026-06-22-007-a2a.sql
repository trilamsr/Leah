CREATE TABLE a2a_peer (
    id           TEXT PRIMARY KEY,
    name         TEXT NOT NULL,
    paired_at    INTEGER NOT NULL,
    last_seen_at INTEGER,
    paused       INTEGER NOT NULL DEFAULT 0,
    scopes       TEXT NOT NULL
);

-- a2a_consent precedes a2a_call: §5.6 declares the FK direction.
-- Shape inferred from spec §5.7 ("Decision persisted in a2a_consent");
-- scope CHECK mirrors vision_consent.scope ('once'|'session'|'always').
CREATE TABLE a2a_consent (
    id           INTEGER PRIMARY KEY,
    peer_id      TEXT NOT NULL REFERENCES a2a_peer(id) ON DELETE CASCADE,
    kind         TEXT NOT NULL,
    scope        TEXT NOT NULL CHECK(scope IN ('once','session','always')),
    granted_at   INTEGER NOT NULL,
    expires_at   INTEGER
);

CREATE TABLE a2a_call (
    id           INTEGER PRIMARY KEY,
    peer_id      TEXT NOT NULL REFERENCES a2a_peer(id) ON DELETE CASCADE,
    at           INTEGER NOT NULL,
    kind         TEXT NOT NULL,
    bytes_in     INTEGER NOT NULL,
    bytes_out    INTEGER NOT NULL,
    consent_ref  INTEGER REFERENCES a2a_consent(id),
    outcome      TEXT NOT NULL CHECK(outcome IN ('ok','denied','error'))
);

CREATE TABLE mcp_token (
    id           INTEGER PRIMARY KEY,
    name         TEXT NOT NULL,
    hash         BLOB NOT NULL,
    scopes       TEXT NOT NULL,
    issued_at    INTEGER NOT NULL,
    revoked_at   INTEGER
);
