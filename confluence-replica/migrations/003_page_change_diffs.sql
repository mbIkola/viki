CREATE TABLE IF NOT EXISTS page_change_diffs (
  run_id BIGINT NOT NULL REFERENCES sync_runs(run_id) ON DELETE CASCADE,
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
  excerpts_jsonb JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (run_id, page_id)
);

CREATE INDEX IF NOT EXISTS idx_page_change_diffs_created ON page_change_diffs (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_page_change_diffs_page ON page_change_diffs (page_id, created_at DESC);
