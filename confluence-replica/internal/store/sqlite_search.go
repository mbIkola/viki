package store

import (
	"context"
	"fmt"
	"strings"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
)

func (s *SQLiteStore) SearchLexical(ctx context.Context, query string, limit int) ([]LexicalSearchRow, error) {
	query = strings.TrimSpace(query)
	if query == "" || limit <= 0 {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			c.page_id,
			c.chunk_id,
			c.version_number,
			p.title,
			snippet(page_chunks_fts, 0, '[', ']', ' ... ', 18) AS snippet,
			rank
		FROM page_chunks_fts
		JOIN page_chunks c ON c.chunk_rowid = page_chunks_fts.rowid
		JOIN pages p ON p.page_id = c.page_id
		WHERE page_chunks_fts MATCH ?
		ORDER BY rank
		LIMIT ?
	`, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]LexicalSearchRow, 0)
	for rows.Next() {
		var item LexicalSearchRow
		if err := rows.Scan(&item.PageID, &item.ChunkID, &item.Version, &item.Title, &item.Snippet, &item.RankValue); err != nil {
			return nil, err
		}
		item.Rank = len(out) + 1
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) SearchSemantic(ctx context.Context, embedding []float32, limit int) ([]SemanticSearchRow, error) {
	if len(embedding) == 0 || limit <= 0 {
		return nil, nil
	}
	if len(embedding) != s.profile.EmbeddingDimension {
		return nil, fmt.Errorf("reindex required: query embedding dimension %d does not match profile dimension %d", len(embedding), s.profile.EmbeddingDimension)
	}
	blob, err := sqlite_vec.SerializeFloat32(embedding)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			c.page_id,
			c.chunk_id,
			c.version_number,
			p.title,
			substr(c.chunk_text, 1, 240) AS snippet,
			v.distance
		FROM chunk_vectors v
		JOIN page_chunks c ON c.chunk_rowid = v.rowid
		JOIN pages p ON p.page_id = c.page_id
		WHERE v.embedding MATCH ?
		  AND v.k = ?
		ORDER BY v.distance ASC, c.page_id ASC, c.chunk_id ASC
	`, blob, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]SemanticSearchRow, 0)
	for rows.Next() {
		var item SemanticSearchRow
		if err := rows.Scan(&item.PageID, &item.ChunkID, &item.Version, &item.Title, &item.Snippet, &item.Distance); err != nil {
			return nil, err
		}
		item.Rank = len(out) + 1
		item.EmbeddingModel = s.profile.EmbeddingModel
		out = append(out, item)
	}
	return out, rows.Err()
}
