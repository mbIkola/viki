package store

import (
	"context"
	"encoding/json"
)

func (s *SQLiteStore) GetPageCurrent(ctx context.Context, pageID string) (PageDocument, error) {
	return s.GetPageVersion(ctx, pageID, 0)
}

func (s *SQLiteStore) GetPageVersion(ctx context.Context, pageID string, version int) (PageDocument, error) {
	targetVersion := version
	if targetVersion <= 0 {
		if err := s.db.QueryRowContext(ctx, `SELECT current_version FROM pages WHERE page_id = ?`, pageID).Scan(&targetVersion); err != nil {
			return PageDocument{}, err
		}
	}

	var (
		doc       PageDocument
		labelsRaw string
		createdAt string
		updatedAt string
		fetchedAt string
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT
			p.page_id,
			p.space_key,
			p.title,
			p.parent_page_id,
			p.status,
			p.current_version,
			p.created_at,
			p.updated_at,
			p.labels_json,
			v.version_number,
			v.body_raw,
			v.body_norm,
			v.body_hash,
			v.fetched_at
		FROM pages p
		JOIN page_versions v
			ON v.page_id = p.page_id
			AND v.version_number = ?
		WHERE p.page_id = ?
	`, targetVersion, pageID).Scan(
		&doc.PageID,
		&doc.SpaceKey,
		&doc.Title,
		&doc.ParentPageID,
		&doc.Status,
		&doc.CurrentVer,
		&createdAt,
		&updatedAt,
		&labelsRaw,
		&doc.Version,
		&doc.BodyRaw,
		&doc.BodyNorm,
		&doc.BodyHash,
		&fetchedAt,
	)
	if err != nil {
		return PageDocument{}, err
	}
	doc.CreatedAt = mustParseTime(createdAt)
	doc.UpdatedAt = mustParseTime(updatedAt)
	doc.FetchedAt = mustParseTime(fetchedAt)
	if labelsRaw != "" {
		_ = json.Unmarshal([]byte(labelsRaw), &doc.Labels)
	}
	return doc, nil
}

func (s *SQLiteStore) GetChunk(ctx context.Context, chunkID string) (ChunkDocument, error) {
	var doc ChunkDocument
	err := s.db.QueryRowContext(ctx, `
		SELECT c.chunk_id, c.page_id, c.version_number, p.title, c.chunk_text, c.chunk_hash, c.token_count
		FROM page_chunks c
		JOIN pages p ON p.page_id = c.page_id
		WHERE c.chunk_id = ?
	`, chunkID).Scan(&doc.ChunkID, &doc.PageID, &doc.Version, &doc.Title, &doc.ChunkText, &doc.ChunkHash, &doc.TokenCount)
	if err != nil {
		return ChunkDocument{}, err
	}
	return doc, nil
}

func (s *SQLiteStore) GetTree(ctx context.Context, rootPageID string, depth int, limit int) ([]TreeNode, error) {
	if depth < 0 {
		depth = 0
	}
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `
		WITH RECURSIVE tree AS (
			SELECT p.page_id, p.title, p.parent_page_id, p.current_version, p.updated_at, 0 AS depth
			FROM pages p
			WHERE p.page_id = ?
			UNION ALL
			SELECT c.page_id, c.title, c.parent_page_id, c.current_version, c.updated_at, t.depth + 1
			FROM pages c
			JOIN tree t ON c.parent_page_id = t.page_id
			WHERE t.depth < ?
		)
		SELECT page_id, title, parent_page_id, current_version, updated_at, depth
		FROM tree
		ORDER BY depth ASC, updated_at DESC
		LIMIT ?
	`, rootPageID, depth, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]TreeNode, 0)
	for rows.Next() {
		var (
			item      TreeNode
			updatedAt string
		)
		if err := rows.Scan(&item.PageID, &item.Title, &item.ParentPageID, &item.CurrentVer, &updatedAt, &item.Depth); err != nil {
			return nil, err
		}
		item.UpdatedAt = mustParseTime(updatedAt)
		out = append(out, item)
	}
	return out, rows.Err()
}
