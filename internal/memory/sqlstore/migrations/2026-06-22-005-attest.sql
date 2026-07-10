CREATE TABLE attest_record (
    id          INTEGER PRIMARY KEY,
    subject     TEXT NOT NULL,
    state       TEXT NOT NULL,
    verified_at INTEGER NOT NULL,
    next_recheck INTEGER NOT NULL,
    signed_by   TEXT NOT NULL,
    reason      TEXT
);

CREATE INDEX idx_attest_subject_at ON attest_record(subject, verified_at DESC);
