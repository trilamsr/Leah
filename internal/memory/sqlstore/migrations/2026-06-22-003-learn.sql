CREATE TABLE learn_observation (
    id          INTEGER PRIMARY KEY,
    at          INTEGER NOT NULL,
    kind        TEXT NOT NULL,
    payload     BLOB,
    ctx_hash    BLOB
);

CREATE TABLE learn_decay (
    id            INTEGER PRIMARY KEY,
    kind          TEXT NOT NULL,
    half_life_s   INTEGER NOT NULL,
    hard_expire_s INTEGER NOT NULL
);

CREATE TABLE learn_recommendation (
    id          INTEGER PRIMARY KEY,
    kind        TEXT NOT NULL,
    body        TEXT NOT NULL,
    action_ref  TEXT NOT NULL,
    score       REAL NOT NULL,
    confidence  REAL NOT NULL,
    decay_id    INTEGER NOT NULL REFERENCES learn_decay(id),
    surfaced_at INTEGER,
    expires_at  INTEGER NOT NULL,
    state       TEXT NOT NULL
);

CREATE TABLE learn_experiment (
    id          INTEGER PRIMARY KEY,
    kind        TEXT NOT NULL,
    arm_a       TEXT NOT NULL,
    arm_b       TEXT NOT NULL,
    impressions_a INTEGER NOT NULL DEFAULT 0,
    impressions_b INTEGER NOT NULL DEFAULT 0,
    wins_a      INTEGER NOT NULL DEFAULT 0,
    wins_b      INTEGER NOT NULL DEFAULT 0,
    locked      INTEGER NOT NULL DEFAULT 0,
    locked_arm  TEXT
);

CREATE TABLE anti_recommend (
    kind        TEXT NOT NULL,
    reason      TEXT NOT NULL,
    added_at    INTEGER NOT NULL,
    source      TEXT NOT NULL CHECK(source IN ('operator','auto','spec')),
    PRIMARY KEY(kind, source)
);

INSERT INTO anti_recommend(kind, reason, added_at, source) VALUES
  ('wake-word-on', 'spec §3.6: operator must opt in', strftime('%s','now'), 'spec');
