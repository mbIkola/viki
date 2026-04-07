package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"confluence-replica/internal/logx"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Page struct {
	PageID       string
	SpaceKey     string
	Title        string
	ParentPageID string
	CurrentVer   int
	UpdatedAt    time.Time
	CreatedAt    time.Time
	PathHash     string
	Tags         []string
	Status       string
}

type PageVersion struct {
	PageID     string
	Version    int
	AuthorID   string
	BodyRaw    string
	BodyNorm   string
	BodyHash   string
	Title      string
	ParentPage string
	FetchedAt  time.Time
}

type Chunk struct {
	PageID     string
	Version    int
	ChunkID    string
	ChunkText  string
	ChunkHash  string
	TokenCount int
	Embedding  []float32
}

type PageState struct {
	PageID       string
	Title        string
	ParentPageID string
	Version      int
	BodyNormHash string
}

type ChangeEvent struct {
	RunID      int64
	PageID     string
	Type       string
	OldVersion int
	NewVersion int
	OldParent  string
	NewParent  string
	OldTitle   string
	NewTitle   string
	Summary    string
}

type SearchRow struct {
	PageID         string
	ChunkID        string
	Version        int
	Title          string
	Snippet        string
	FTSScore       float64
	SemanticScore  float64
	Freshness      float64
	ParentDistance float64
	VersionRecency float64
	TagMatch       float64
}

type PageDocument struct {
	PageID       string
	SpaceKey     string
	Title        string
	ParentPageID string
	Status       string
	CurrentVer   int
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Labels       []string
	Version      int
	BodyRaw      string
	BodyNorm     string
	BodyHash     string
	FetchedAt    time.Time
}

type ChunkDocument struct {
	ChunkID    string
	PageID     string
	Version    int
	Title      string
	ChunkText  string
	ChunkHash  string
	TokenCount int
}

type TreeNode struct {
	PageID       string
	Title        string
	ParentPageID string
	CurrentVer   int
	Depth        int
	UpdatedAt    time.Time
}

type Store interface {
	BeginSyncRun(ctx context.Context, mode string) (int64, error)
	FinishSyncRun(ctx context.Context, runID int64, status string, stats map[string]any) error
	UpsertPageWithVersion(ctx context.Context, p Page, v PageVersion, chunks []Chunk) error
	ListCurrentStates(ctx context.Context) ([]PageState, error)
	InsertChangeEvents(ctx context.Context, events []ChangeEvent) error
	SaveDigest(ctx context.Context, date time.Time, markdown string, stats map[string]any) error
	GetDigest(ctx context.Context, date time.Time) (string, error)
	ListChangeEventsForDate(ctx context.Context, date time.Time) ([]ChangeEvent, error)
	SearchHybrid(ctx context.Context, query string, embedding []float32, limit int) ([]SearchRow, error)
	GetPageCurrent(ctx context.Context, pageID string) (PageDocument, error)
	GetPageVersion(ctx context.Context, pageID string, version int) (PageDocument, error)
	GetChunk(ctx context.Context, chunkID string) (ChunkDocument, error)
	GetTree(ctx context.Context, rootPageID string, depth int, limit int) ([]TreeNode, error)
}

type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(ctx context.Context, dsn string) (*PostgresStore, error) {
	if dsn == "" {
		return nil, errors.New("database dsn is required")
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	st := &PostgresStore{pool: pool}
	logx.Infof("[store] postgres_connected")
	return st, nil
}

func (s *PostgresStore) Close() {
	if s.pool != nil {
		s.pool.Close()
	}
}

func (s *PostgresStore) BeginSyncRun(ctx context.Context, mode string) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO sync_runs (mode, status, started_at)
		VALUES ($1, 'running', now())
		RETURNING run_id
	`, mode).Scan(&id)
	if err == nil {
		logx.Infof("[store] sync_run_started mode=%s run_id=%d", mode, id)
	}
	return id, err
}

func (s *PostgresStore) FinishSyncRun(ctx context.Context, runID int64, status string, stats map[string]any) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE sync_runs
		SET status=$2, finished_at=now(), stats_jsonb=$3::jsonb
		WHERE run_id=$1
	`, runID, status, toJSON(stats))
	if err == nil {
		logx.Infof("[store] sync_run_finished run_id=%d status=%s stats=%s", runID, status, toJSON(stats))
	}
	return err
}

func (s *PostgresStore) UpsertPageWithVersion(ctx context.Context, p Page, v PageVersion, chunks []Chunk) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx, `
		INSERT INTO pages(page_id, space_key, title, parent_page_id, status, current_version, created_at, updated_at, path_hash, labels_jsonb)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb)
		ON CONFLICT (page_id) DO UPDATE SET
			space_key=excluded.space_key,
			title=excluded.title,
			parent_page_id=excluded.parent_page_id,
			status=excluded.status,
			current_version=excluded.current_version,
			updated_at=excluded.updated_at,
			path_hash=excluded.path_hash,
			labels_jsonb=excluded.labels_jsonb
	`, p.PageID, p.SpaceKey, p.Title, nullIfEmpty(p.ParentPageID), p.Status, p.CurrentVer, p.CreatedAt, p.UpdatedAt, p.PathHash, toJSON(p.Tags))
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO page_versions(page_id, version_number, author_id, fetched_at, body_raw, body_norm, body_hash, title, parent_page_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (page_id, version_number) DO NOTHING
	`, v.PageID, v.Version, nullIfEmpty(v.AuthorID), v.FetchedAt, v.BodyRaw, v.BodyNorm, v.BodyHash, v.Title, nullIfEmpty(v.ParentPage))
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `DELETE FROM page_chunks WHERE page_id=$1 AND version_number=$2`, v.PageID, v.Version)
	if err != nil {
		return err
	}

	for _, c := range chunks {
		_, err = tx.Exec(ctx, `
			INSERT INTO page_chunks(page_id, version_number, chunk_id, chunk_text, chunk_hash, token_count, fts_tsv)
			VALUES($1,$2,$3,$4,$5,$6,to_tsvector('english', $4))
		`, c.PageID, c.Version, c.ChunkID, c.ChunkText, c.ChunkHash, c.TokenCount)
		if err != nil {
			return err
		}
		if len(c.Embedding) > 0 {
			_, err = tx.Exec(ctx, `
				INSERT INTO chunk_embeddings(page_id, version_number, chunk_id, embedding, embedding_dim)
				VALUES($1,$2,$3,$4::vector,$5)
				ON CONFLICT (page_id, version_number, chunk_id) DO UPDATE SET
					embedding=excluded.embedding,
					embedding_dim=excluded.embedding_dim
			`, c.PageID, c.Version, c.ChunkID, vectorLiteral(c.Embedding), len(c.Embedding))
			if err != nil {
				return err
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	logx.Debugf("[store] upsert_page page_id=%s version=%d chunks=%d tables=pages,page_versions,page_chunks,chunk_embeddings", p.PageID, v.Version, len(chunks))
	return nil
}

func (s *PostgresStore) ListCurrentStates(ctx context.Context) ([]PageState, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT p.page_id, p.title, COALESCE(p.parent_page_id,''), p.current_version, COALESCE(v.body_hash,'')
		FROM pages p
		LEFT JOIN page_versions v ON v.page_id = p.page_id AND v.version_number = p.current_version
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]PageState, 0)
	for rows.Next() {
		var s0 PageState
		if err := rows.Scan(&s0.PageID, &s0.Title, &s0.ParentPageID, &s0.Version, &s0.BodyNormHash); err != nil {
			return nil, err
		}
		out = append(out, s0)
	}
	return out, rows.Err()
}

func (s *PostgresStore) InsertChangeEvents(ctx context.Context, events []ChangeEvent) error {
	if len(events) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, ev := range events {
		batch.Queue(`
			INSERT INTO change_events(
				run_id, page_id, event_type, old_version, new_version, old_parent, new_parent, old_title, new_title, summary_short
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		`, ev.RunID, ev.PageID, ev.Type, zeroToNull(ev.OldVersion), zeroToNull(ev.NewVersion), nullIfEmpty(ev.OldParent), nullIfEmpty(ev.NewParent), nullIfEmpty(ev.OldTitle), nullIfEmpty(ev.NewTitle), ev.Summary)
	}
	br := s.pool.SendBatch(ctx, batch)
	defer br.Close()
	for range events {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	logx.Debugf("[store] change_events_inserted count=%d table=change_events", len(events))
	return nil
}

func (s *PostgresStore) SaveDigest(ctx context.Context, date time.Time, markdown string, stats map[string]any) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO daily_digests(digest_date, generated_at, markdown, stats_jsonb)
		VALUES ($1, now(), $2, $3::jsonb)
		ON CONFLICT (digest_date) DO UPDATE SET
			generated_at=excluded.generated_at,
			markdown=excluded.markdown,
			stats_jsonb=excluded.stats_jsonb
	`, date.Format("2006-01-02"), markdown, toJSON(stats))
	if err == nil {
		logx.Infof("[store] digest_saved date=%s bytes=%d table=daily_digests", date.Format("2006-01-02"), len(markdown))
	}
	return err
}

func (s *PostgresStore) GetDigest(ctx context.Context, date time.Time) (string, error) {
	var md string
	err := s.pool.QueryRow(ctx, `SELECT markdown FROM daily_digests WHERE digest_date=$1`, date.Format("2006-01-02")).Scan(&md)
	if err != nil {
		return "", err
	}
	return md, nil
}

func (s *PostgresStore) GetPageCurrent(ctx context.Context, pageID string) (PageDocument, error) {
	return s.GetPageVersion(ctx, pageID, 0)
}

func (s *PostgresStore) GetPageVersion(ctx context.Context, pageID string, version int) (PageDocument, error) {
	query := `
		SELECT
			p.page_id,
			p.space_key,
			p.title,
			COALESCE(p.parent_page_id, ''),
			p.status,
			p.current_version,
			p.created_at,
			p.updated_at,
			p.labels_jsonb,
			v.version_number,
			v.body_raw,
			v.body_norm,
			v.body_hash,
			v.fetched_at
		FROM pages p
		JOIN page_versions v ON v.page_id = p.page_id
		WHERE p.page_id = $1
		  AND v.version_number = CASE WHEN $2 <= 0 THEN p.current_version ELSE $2 END
	`
	var (
		doc       PageDocument
		labelsRaw []byte
	)
	err := s.pool.QueryRow(ctx, query, pageID, version).Scan(
		&doc.PageID,
		&doc.SpaceKey,
		&doc.Title,
		&doc.ParentPageID,
		&doc.Status,
		&doc.CurrentVer,
		&doc.CreatedAt,
		&doc.UpdatedAt,
		&labelsRaw,
		&doc.Version,
		&doc.BodyRaw,
		&doc.BodyNorm,
		&doc.BodyHash,
		&doc.FetchedAt,
	)
	if err != nil {
		return PageDocument{}, err
	}
	if len(labelsRaw) > 0 {
		_ = json.Unmarshal(labelsRaw, &doc.Labels)
	}
	return doc, nil
}

func (s *PostgresStore) GetChunk(ctx context.Context, chunkID string) (ChunkDocument, error) {
	var doc ChunkDocument
	err := s.pool.QueryRow(ctx, `
		SELECT c.chunk_id, c.page_id, c.version_number, p.title, c.chunk_text, c.chunk_hash, c.token_count
		FROM page_chunks c
		JOIN pages p ON p.page_id = c.page_id
		WHERE c.chunk_id = $1
	`, chunkID).Scan(
		&doc.ChunkID,
		&doc.PageID,
		&doc.Version,
		&doc.Title,
		&doc.ChunkText,
		&doc.ChunkHash,
		&doc.TokenCount,
	)
	if err != nil {
		return ChunkDocument{}, err
	}
	return doc, nil
}

func (s *PostgresStore) GetTree(ctx context.Context, rootPageID string, depth int, limit int) ([]TreeNode, error) {
	if depth < 0 {
		depth = 0
	}
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.pool.Query(ctx, `
		WITH RECURSIVE tree AS (
			SELECT p.page_id, p.title, COALESCE(p.parent_page_id, '') AS parent_page_id, p.current_version, p.updated_at, 0 AS depth
			FROM pages p
			WHERE p.page_id = $1
			UNION ALL
			SELECT c.page_id, c.title, COALESCE(c.parent_page_id, '') AS parent_page_id, c.current_version, c.updated_at, t.depth + 1
			FROM pages c
			JOIN tree t ON c.parent_page_id = t.page_id
			WHERE t.depth < $2
		)
		SELECT page_id, title, parent_page_id, current_version, updated_at, depth
		FROM tree
		ORDER BY depth ASC, updated_at DESC
		LIMIT $3
	`, rootPageID, depth, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]TreeNode, 0)
	for rows.Next() {
		var n TreeNode
		if err := rows.Scan(&n.PageID, &n.Title, &n.ParentPageID, &n.CurrentVer, &n.UpdatedAt, &n.Depth); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *PostgresStore) ListChangeEventsForDate(ctx context.Context, date time.Time) ([]ChangeEvent, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT run_id, page_id, event_type, COALESCE(old_version, 0), COALESCE(new_version, 0),
			COALESCE(old_parent, ''), COALESCE(new_parent, ''), COALESCE(old_title, ''), COALESCE(new_title, ''), summary_short
		FROM change_events
		WHERE DATE(created_at) = $1
		ORDER BY created_at ASC, event_id ASC
	`, date.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]ChangeEvent, 0)
	for rows.Next() {
		var e ChangeEvent
		if err := rows.Scan(&e.RunID, &e.PageID, &e.Type, &e.OldVersion, &e.NewVersion, &e.OldParent, &e.NewParent, &e.OldTitle, &e.NewTitle, &e.Summary); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *PostgresStore) SearchHybrid(ctx context.Context, query string, embedding []float32, limit int) ([]SearchRow, error) {
	if len(embedding) > 0 {
		return s.searchHybridWithEmbedding(ctx, query, embedding, limit)
	}
	return s.searchFTSOnly(ctx, query, limit)
}

func (s *PostgresStore) searchFTSOnly(ctx context.Context, query string, limit int) ([]SearchRow, error) {
	rows, err := s.pool.Query(ctx, `
		WITH fts AS (
			SELECT
				c.page_id,
				c.version_number,
				c.chunk_id,
				LEFT(c.chunk_text, 240) AS snippet,
				ts_rank_cd(c.fts_tsv, plainto_tsquery('english', $1)) AS fts_score
			FROM page_chunks c
			WHERE c.fts_tsv @@ plainto_tsquery('english', $1)
			ORDER BY fts_score DESC
			LIMIT $2
		)
		SELECT
			f.page_id,
			f.chunk_id,
			f.version_number,
			p.title,
			f.snippet,
			f.fts_score,
			0.0 AS semantic_score,
			LEAST(1.0, GREATEST(0.0, 1.0 - EXTRACT(EPOCH FROM (now() - p.updated_at))/86400/30)) AS freshness,
			0.0 AS parent_distance,
			LEAST(1.0, p.current_version / 100.0) AS version_recency,
			0.0 AS tag_match
		FROM fts f
		JOIN pages p ON p.page_id = f.page_id
		ORDER BY f.fts_score DESC
	`, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]SearchRow, 0)
	for rows.Next() {
		var r SearchRow
		if err := rows.Scan(&r.PageID, &r.ChunkID, &r.Version, &r.Title, &r.Snippet, &r.FTSScore, &r.SemanticScore, &r.Freshness, &r.ParentDistance, &r.VersionRecency, &r.TagMatch); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *PostgresStore) searchHybridWithEmbedding(ctx context.Context, query string, embedding []float32, limit int) ([]SearchRow, error) {
	vector := vectorLiteral(embedding)
	dim := len(embedding)
	rows, err := s.pool.Query(ctx, `
		WITH fts AS (
			SELECT
				c.page_id,
				c.version_number,
				c.chunk_id,
				LEFT(c.chunk_text, 240) AS snippet,
				ts_rank_cd(c.fts_tsv, plainto_tsquery('english', $1)) AS fts_score
			FROM page_chunks c
			WHERE c.fts_tsv @@ plainto_tsquery('english', $1)
			ORDER BY fts_score DESC
			LIMIT $3
		),
		sem AS (
			SELECT
				ce.page_id,
				ce.version_number,
				ce.chunk_id,
				LEFT(c.chunk_text, 240) AS snippet,
				(1 - (ce.embedding <=> $2::vector)) AS semantic_score
			FROM chunk_embeddings ce
			JOIN page_chunks c
				ON c.page_id = ce.page_id
				AND c.version_number = ce.version_number
				AND c.chunk_id = ce.chunk_id
			WHERE ce.embedding_dim = $4
			ORDER BY ce.embedding <=> $2::vector
			LIMIT $3
		),
		combined AS (
			SELECT
				COALESCE(fts.page_id, sem.page_id) AS page_id,
				COALESCE(fts.version_number, sem.version_number) AS version_number,
				COALESCE(fts.chunk_id, sem.chunk_id) AS chunk_id,
				COALESCE(fts.snippet, sem.snippet) AS snippet,
				COALESCE(MAX(fts.fts_score), 0.0) AS fts_score,
				COALESCE(MAX(sem.semantic_score), 0.0) AS semantic_score
			FROM fts
			FULL OUTER JOIN sem ON sem.page_id = fts.page_id AND sem.version_number = fts.version_number
			GROUP BY 1,2,3,4
		)
		SELECT
			c.page_id,
			c.chunk_id,
			c.version_number,
			p.title,
			c.snippet,
			c.fts_score,
			c.semantic_score,
			LEAST(1.0, GREATEST(0.0, 1.0 - EXTRACT(EPOCH FROM (now() - p.updated_at))/86400/30)) AS freshness,
			0.0 AS parent_distance,
			LEAST(1.0, p.current_version / 100.0) AS version_recency,
			0.0 AS tag_match
		FROM combined c
		JOIN pages p ON p.page_id = c.page_id
		ORDER BY (c.fts_score + c.semantic_score) DESC
		LIMIT $3
	`, query, vector, limit, dim)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]SearchRow, 0)
	for rows.Next() {
		var r SearchRow
		if err := rows.Scan(&r.PageID, &r.ChunkID, &r.Version, &r.Title, &r.Snippet, &r.FTSScore, &r.SemanticScore, &r.Freshness, &r.ParentDistance, &r.VersionRecency, &r.TagMatch); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func vectorLiteral(v []float32) string {
	if len(v) == 0 {
		return "[]"
	}
	var b strings.Builder
	b.WriteByte('[')
	for i, x := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(x), 'f', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func zeroToNull(n int) any {
	if n == 0 {
		return nil
	}
	return n
}

func toJSON(v any) string {
	if v == nil {
		return "{}"
	}
	b, err := json.Marshal(v)
	if err != nil || len(b) == 0 {
		return "{}"
	}
	return string(b)
}
