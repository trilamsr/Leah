CREATE TABLE budget_bucket (
    name        TEXT PRIMARY KEY,
    cap         INTEGER NOT NULL,
    window      TEXT NOT NULL,
    enabled     INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE budget_sample (
    bucket      TEXT NOT NULL,
    at          INTEGER NOT NULL,
    spent       INTEGER NOT NULL,
    PRIMARY KEY(bucket, at)
);

CREATE INDEX idx_budget_sample_bucket_at ON budget_sample(bucket, at DESC);

INSERT INTO budget_bucket(name, cap, window, enabled) VALUES
  ('cloud.llm.tokens',     0,        'day', 1),
  ('cloud.embed.bytes',    52428800, 'day', 1),
  ('cloud.stt.seconds',    900,      'day', 1),
  ('cloud.tts.chars',      25000,    'day', 1),
  ('cloud.vision.bytes',   31457280, 'day', 1),
  ('peer.a2a.tokens',      5000,     'day', 1),
  ('plugin.network.bytes', 52428800, 'day', 1);
