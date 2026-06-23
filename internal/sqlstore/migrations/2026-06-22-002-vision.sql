CREATE TABLE vision_event (
    id          INTEGER PRIMARY KEY,
    at          INTEGER NOT NULL,
    mode        TEXT NOT NULL CHECK(mode IN ('screenshot','selection','live_screen','live_camera','ocr')),
    sent_to_cloud INTEGER NOT NULL DEFAULT 0,
    bytes       INTEGER NOT NULL,
    prompt      TEXT,
    thumb_path  TEXT,
    consent_ref TEXT
);

CREATE TABLE vision_consent (
    id          INTEGER PRIMARY KEY,
    mode        TEXT NOT NULL,
    granted_at  INTEGER NOT NULL,
    expires_at  INTEGER,
    scope       TEXT NOT NULL
);
