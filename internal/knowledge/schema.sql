CREATE TABLE IF NOT EXISTS schema_meta (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS entities (
  kind         TEXT NOT NULL,
  key          TEXT NOT NULL,
  display      TEXT NOT NULL,
  aliases_json TEXT NOT NULL DEFAULT '[]',
  first_seen   INTEGER NOT NULL,
  last_touched INTEGER NOT NULL,
  PRIMARY KEY (kind, key)
);
CREATE TABLE IF NOT EXISTS entity_items (
  kind     TEXT NOT NULL,
  key      TEXT NOT NULL,
  source   TEXT NOT NULL,
  item_id  TEXT NOT NULL,
  ts       INTEGER NOT NULL,
  PRIMARY KEY (kind, key, source, item_id)
);
CREATE INDEX IF NOT EXISTS idx_entity_items_ts ON entity_items(ts);
CREATE TABLE IF NOT EXISTS knowledge_chunks (
  id   TEXT PRIMARY KEY,
  text TEXT NOT NULL
);
