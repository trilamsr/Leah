CREATE TABLE plugin (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    version     TEXT NOT NULL,
    installed_at INTEGER NOT NULL,
    enabled     INTEGER NOT NULL DEFAULT 1,
    manifest    BLOB NOT NULL,
    bundle_path TEXT NOT NULL,
    attest_state TEXT NOT NULL
);

CREATE TABLE plugin_log (
    id          INTEGER PRIMARY KEY,
    plugin_id   TEXT NOT NULL REFERENCES plugin(id) ON DELETE CASCADE,
    at          INTEGER NOT NULL,
    level       INTEGER NOT NULL,
    msg         TEXT NOT NULL,
    kv          BLOB
);

CREATE TABLE plugin_quota (
    plugin_id   TEXT NOT NULL,
    window_at   INTEGER NOT NULL,
    rpcs        INTEGER NOT NULL DEFAULT 0,
    bytes       INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY(plugin_id, window_at)
);
