CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS pages (
  page_id TEXT PRIMARY KEY,
  space_key TEXT NOT NULL,
  title TEXT NOT NULL,
  parent_page_id TEXT NULL,
  status TEXT NOT NULL,
  current_version INTEGER NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  path_hash TEXT NOT NULL DEFAULT '',
  labels_jsonb JSONB NOT NULL DEFAULT '[]'::jsonb
);

CREATE TABLE IF NOT EXISTS page_versions (
  page_id TEXT NOT NULL,
  version_number INTEGER NOT NULL,
  author_id TEXT NULL,
  fetched_at TIMESTAMPTZ NOT NULL,
  body_raw TEXT NOT NULL,
  body_norm TEXT NOT NULL,
  body_hash TEXT NOT NULL,
  title TEXT NOT NULL,
  parent_page_id TEXT NULL,
  PRIMARY KEY (page_id, version_number),
  FOREIGN KEY (page_id) REFERENCES pages(page_id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS page_chunks (
  page_id TEXT NOT NULL,
  version_number INTEGER NOT NULL,
  chunk_id TEXT NOT NULL,
  chunk_text TEXT NOT NULL,
  chunk_hash TEXT NOT NULL,
  token_count INTEGER NOT NULL,
  fts_tsv tsvector NOT NULL,
  PRIMARY KEY (page_id, version_number, chunk_id),
  FOREIGN KEY (page_id, version_number) REFERENCES page_versions(page_id, version_number) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS chunk_embeddings (
  page_id TEXT NOT NULL,
  version_number INTEGER NOT NULL,
  chunk_id TEXT NOT NULL,
  embedding vector(1536) NOT NULL,
  PRIMARY KEY (page_id, version_number, chunk_id),
  FOREIGN KEY (page_id, version_number, chunk_id) REFERENCES page_chunks(page_id, version_number, chunk_id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS sync_runs (
  run_id BIGSERIAL PRIMARY KEY,
  mode TEXT NOT NULL,
  status TEXT NOT NULL,
  started_at TIMESTAMPTZ NOT NULL,
  finished_at TIMESTAMPTZ NULL,
  stats_jsonb JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE TABLE IF NOT EXISTS change_events (
  event_id BIGSERIAL PRIMARY KEY,
  run_id BIGINT NOT NULL REFERENCES sync_runs(run_id) ON DELETE CASCADE,
  page_id TEXT NOT NULL,
  event_type TEXT NOT NULL,
  old_version INTEGER NULL,
  new_version INTEGER NULL,
  old_parent TEXT NULL,
  new_parent TEXT NULL,
  old_title TEXT NULL,
  new_title TEXT NULL,
  summary_short TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS daily_digests (
  digest_date DATE PRIMARY KEY,
  generated_at TIMESTAMPTZ NOT NULL,
  markdown TEXT NOT NULL,
  stats_jsonb JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX IF NOT EXISTS idx_page_chunks_fts ON page_chunks USING GIN (fts_tsv);
CREATE INDEX IF NOT EXISTS idx_pages_parent ON pages (parent_page_id);
CREATE INDEX IF NOT EXISTS idx_pages_updated ON pages (updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_change_events_created ON change_events (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_page_versions_lookup ON page_versions (page_id, version_number DESC);
CREATE INDEX IF NOT EXISTS idx_chunk_embeddings_hnsw ON chunk_embeddings USING hnsw (embedding vector_cosine_ops);
