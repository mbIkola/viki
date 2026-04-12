CREATE TABLE IF NOT EXISTS replica_meta (
  singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
  schema_version INTEGER NOT NULL,
  embedding_provider TEXT NOT NULL,
  embedding_model TEXT NOT NULL,
  embedding_dimension INTEGER NOT NULL,
  chunking_version TEXT NOT NULL,
  embedding_normalization TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS pages (
  page_id TEXT PRIMARY KEY,
  space_key TEXT NOT NULL,
  title TEXT NOT NULL,
  parent_page_id TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL,
  current_version INTEGER NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  path_hash TEXT NOT NULL DEFAULT '',
  labels_json TEXT NOT NULL DEFAULT '[]'
);

CREATE TABLE IF NOT EXISTS page_versions (
  page_id TEXT NOT NULL,
  version_number INTEGER NOT NULL,
  author_id TEXT NOT NULL DEFAULT '',
  fetched_at TEXT NOT NULL,
  body_raw TEXT NOT NULL,
  body_norm TEXT NOT NULL,
  body_hash TEXT NOT NULL,
  title TEXT NOT NULL,
  parent_page_id TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (page_id, version_number),
  FOREIGN KEY (page_id) REFERENCES pages(page_id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS page_chunks (
  chunk_rowid INTEGER PRIMARY KEY,
  page_id TEXT NOT NULL,
  version_number INTEGER NOT NULL,
  chunk_id TEXT NOT NULL UNIQUE,
  chunk_text TEXT NOT NULL,
  chunk_hash TEXT NOT NULL,
  token_count INTEGER NOT NULL,
  FOREIGN KEY (page_id, version_number) REFERENCES page_versions(page_id, version_number) ON DELETE CASCADE
);

CREATE VIRTUAL TABLE IF NOT EXISTS page_chunks_fts USING fts5(
  chunk_text,
  content='page_chunks',
  content_rowid='chunk_rowid',
  tokenize='porter unicode61'
);

CREATE TRIGGER IF NOT EXISTS page_chunks_ai AFTER INSERT ON page_chunks BEGIN
  INSERT INTO page_chunks_fts(rowid, chunk_text) VALUES (new.chunk_rowid, new.chunk_text);
END;

CREATE TRIGGER IF NOT EXISTS page_chunks_ad AFTER DELETE ON page_chunks BEGIN
  INSERT INTO page_chunks_fts(page_chunks_fts, rowid, chunk_text) VALUES ('delete', old.chunk_rowid, old.chunk_text);
END;

CREATE TRIGGER IF NOT EXISTS page_chunks_au AFTER UPDATE ON page_chunks BEGIN
  INSERT INTO page_chunks_fts(page_chunks_fts, rowid, chunk_text) VALUES ('delete', old.chunk_rowid, old.chunk_text);
  INSERT INTO page_chunks_fts(rowid, chunk_text) VALUES (new.chunk_rowid, new.chunk_text);
END;

CREATE TABLE IF NOT EXISTS sync_runs (
  run_id INTEGER PRIMARY KEY AUTOINCREMENT,
  mode TEXT NOT NULL,
  status TEXT NOT NULL,
  started_at TEXT NOT NULL,
  finished_at TEXT NULL,
  stats_json TEXT NOT NULL DEFAULT '{}'
);

CREATE TABLE IF NOT EXISTS change_events (
  event_id INTEGER PRIMARY KEY AUTOINCREMENT,
  run_id INTEGER NOT NULL REFERENCES sync_runs(run_id) ON DELETE CASCADE,
  page_id TEXT NOT NULL,
  event_type TEXT NOT NULL,
  old_version INTEGER NULL,
  new_version INTEGER NULL,
  old_parent TEXT NULL,
  new_parent TEXT NULL,
  old_title TEXT NULL,
  new_title TEXT NULL,
  summary_short TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS daily_digests (
  digest_date TEXT PRIMARY KEY,
  generated_at TEXT NOT NULL,
  markdown TEXT NOT NULL,
  stats_json TEXT NOT NULL DEFAULT '{}'
);

CREATE TABLE IF NOT EXISTS page_change_diffs (
  run_id INTEGER NOT NULL REFERENCES sync_runs(run_id) ON DELETE CASCADE,
  page_id TEXT NOT NULL REFERENCES pages(page_id) ON DELETE CASCADE,
  old_version INTEGER NULL,
  new_version INTEGER NULL,
  change_kind TEXT NOT NULL,
  title_changed BOOLEAN NOT NULL DEFAULT FALSE,
  parent_changed BOOLEAN NOT NULL DEFAULT FALSE,
  body_raw_changed BOOLEAN NOT NULL DEFAULT FALSE,
  body_norm_changed BOOLEAN NOT NULL DEFAULT FALSE,
  body_hash_old TEXT NULL,
  body_hash_new TEXT NULL,
  diagram_change_detected BOOLEAN NOT NULL DEFAULT FALSE,
  diagram_content_unparsed BOOLEAN NOT NULL DEFAULT FALSE,
  summary TEXT NOT NULL DEFAULT '',
  excerpts_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  PRIMARY KEY (run_id, page_id)
);

CREATE INDEX IF NOT EXISTS idx_pages_parent ON pages (parent_page_id);
CREATE INDEX IF NOT EXISTS idx_pages_updated ON pages (updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_page_versions_lookup ON page_versions (page_id, version_number DESC);
CREATE INDEX IF NOT EXISTS idx_change_events_created ON change_events (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_page_change_diffs_created ON page_change_diffs (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_page_change_diffs_page ON page_change_diffs (page_id, created_at DESC);
