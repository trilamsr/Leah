CREATE TABLE supervisor_event (
    id          INTEGER PRIMARY KEY,
    at          INTEGER NOT NULL,
    process     TEXT NOT NULL,
    kind        TEXT NOT NULL,
    pid         INTEGER,
    code        INTEGER,
    reason      TEXT
);

CREATE TABLE supervisor_rss (
    process     TEXT NOT NULL,
    at          INTEGER NOT NULL,
    rss_mb      INTEGER NOT NULL,
    PRIMARY KEY(process, at)
);
