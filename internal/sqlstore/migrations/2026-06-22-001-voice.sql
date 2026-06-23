CREATE TABLE voice_session (
    id            INTEGER PRIMARY KEY,
    started_at    INTEGER NOT NULL,
    ended_at      INTEGER,
    voice_only    INTEGER NOT NULL DEFAULT 0,
    source        TEXT NOT NULL CHECK(source IN ('wake','hotkey','menubar','ptt')),
    end_reason    TEXT CHECK(end_reason IN ('user','timeout','error','barge_exhausted')),
    stt_provider  TEXT NOT NULL,
    tts_provider  TEXT NOT NULL,
    ram_peak_mb   INTEGER,
    bytes_uploaded INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE voice_turn (
    id            INTEGER PRIMARY KEY,
    session_id    INTEGER NOT NULL REFERENCES voice_session(id) ON DELETE CASCADE,
    ord           INTEGER NOT NULL,
    role          TEXT NOT NULL CHECK(role IN ('user','assistant')),
    text          TEXT NOT NULL,
    stt_ms        INTEGER,
    ttfb_ms       INTEGER,
    tts_ms        INTEGER,
    barge_in      INTEGER NOT NULL DEFAULT 0,
    UNIQUE(session_id, ord)
);

CREATE TABLE voice_suppression (
    bundle_id     TEXT PRIMARY KEY,
    learned       INTEGER NOT NULL DEFAULT 0,
    last_seen_at  INTEGER NOT NULL,
    confidence    REAL NOT NULL DEFAULT 0.0
);
