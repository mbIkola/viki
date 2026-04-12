package store

import (
	"context"
	"database/sql"
	"fmt"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
)

func (s *SQLiteStore) BeginSyncRun(ctx context.Context, mode string) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO sync_runs(mode, status, started_at, stats_json)
		VALUES (?, 'running', ?, ?)
	`, mode, currentTimestamp(), "{}")
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *SQLiteStore) FinishSyncRun(ctx context.Context, runID int64, status string, stats map[string]any) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE sync_runs
		SET status = ?, finished_at = ?, stats_json = ?
		WHERE run_id = ?
	`, status, currentTimestamp(), toJSON(stats), runID)
	return err
}

func (s *SQLiteStore) UpsertPageWithVersion(ctx context.Context, p Page, v PageVersion, chunks []Chunk) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO pages(page_id, space_key, title, parent_page_id, status, current_version, created_at, updated_at, path_hash, labels_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(page_id) DO UPDATE SET
			space_key = excluded.space_key,
			title = excluded.title,
			parent_page_id = excluded.parent_page_id,
			status = excluded.status,
			current_version = excluded.current_version,
			created_at = excluded.created_at,
			updated_at = excluded.updated_at,
			path_hash = excluded.path_hash,
			labels_json = excluded.labels_json
	`, p.PageID, p.SpaceKey, p.Title, p.ParentPageID, p.Status, p.CurrentVer, storeTime(p.CreatedAt), storeTime(p.UpdatedAt), p.PathHash, toJSON(p.Tags))
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO page_versions(page_id, version_number, author_id, fetched_at, body_raw, body_norm, body_hash, title, parent_page_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(page_id, version_number) DO UPDATE SET
			author_id = excluded.author_id,
			fetched_at = excluded.fetched_at,
			body_raw = excluded.body_raw,
			body_norm = excluded.body_norm,
			body_hash = excluded.body_hash,
			title = excluded.title,
			parent_page_id = excluded.parent_page_id
	`, v.PageID, v.Version, v.AuthorID, storeTime(v.FetchedAt), v.BodyRaw, v.BodyNorm, v.BodyHash, v.Title, v.ParentPage)
	if err != nil {
		return err
	}

	rowids, err := chunkRowIDsByVersion(ctx, tx, v.PageID, v.Version)
	if err != nil {
		return err
	}
	for _, rowid := range rowids {
		if _, err := tx.ExecContext(ctx, `DELETE FROM chunk_vectors WHERE rowid = ?`, rowid); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM page_chunks WHERE page_id = ? AND version_number = ?`, v.PageID, v.Version); err != nil {
		return err
	}

	for _, c := range chunks {
		if len(c.Embedding) > 0 && len(c.Embedding) != s.profile.EmbeddingDimension {
			return fmt.Errorf("reindex required: chunk embedding dimension %d does not match profile dimension %d", len(c.Embedding), s.profile.EmbeddingDimension)
		}
		res, err := tx.ExecContext(ctx, `
			INSERT INTO page_chunks(page_id, version_number, chunk_id, chunk_text, chunk_hash, token_count)
			VALUES (?, ?, ?, ?, ?, ?)
		`, c.PageID, c.Version, c.ChunkID, c.ChunkText, c.ChunkHash, c.TokenCount)
		if err != nil {
			return err
		}
		rowid, err := res.LastInsertId()
		if err != nil {
			return err
		}
		if len(c.Embedding) > 0 {
			blob, err := sqlite_vec.SerializeFloat32(c.Embedding)
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO chunk_vectors(rowid, embedding) VALUES (?, ?)`, rowid, blob); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

func (s *SQLiteStore) ListCurrentStates(ctx context.Context) ([]PageState, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT p.page_id, p.title, p.parent_page_id, p.current_version, COALESCE(v.body_hash, '')
		FROM pages p
		LEFT JOIN page_versions v
			ON v.page_id = p.page_id
			AND v.version_number = p.current_version
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]PageState, 0)
	for rows.Next() {
		var item PageState
		if err := rows.Scan(&item.PageID, &item.Title, &item.ParentPageID, &item.Version, &item.BodyNormHash); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func chunkRowIDsByVersion(ctx context.Context, tx *sql.Tx, pageID string, version int) ([]int64, error) {
	rows, err := tx.QueryContext(ctx, `SELECT chunk_rowid FROM page_chunks WHERE page_id = ? AND version_number = ?`, pageID, version)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]int64, 0)
	for rows.Next() {
		var rowid int64
		if err := rows.Scan(&rowid); err != nil {
			return nil, err
		}
		out = append(out, rowid)
	}
	return out, rows.Err()
}
