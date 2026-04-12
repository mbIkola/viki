package store

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	_ "github.com/mattn/go-sqlite3"
)

//go:embed schema.sql
var schemaFS embed.FS

var sqliteAutoOnce sync.Once

type IndexProfile struct {
	SchemaVersion          int
	EmbeddingProvider      string
	EmbeddingModel         string
	EmbeddingDimension     int
	ChunkingVersion        string
	EmbeddingNormalization string
}

type SQLiteStore struct {
	db      *sql.DB
	path    string
	profile IndexProfile
}

func NewSQLiteStore(ctx context.Context, path string, profile IndexProfile) (*SQLiteStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("database path is required")
	}
	if profile.SchemaVersion <= 0 {
		return nil, errors.New("schema version must be positive")
	}
	if profile.EmbeddingDimension <= 0 {
		return nil, errors.New("embedding dimension must be positive")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	sqliteAutoOnce.Do(sqlite_vec.Auto)

	db, err := sql.Open("sqlite3", sqliteDSN(path))
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := configureSQLite(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := bootstrapSchema(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := ensureVectorTable(ctx, db, profile); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := ensureIndexProfile(ctx, db, profile); err != nil {
		_ = db.Close()
		return nil, err
	}

	return &SQLiteStore{
		db:      db,
		path:    path,
		profile: profile,
	}, nil
}

func (s *SQLiteStore) Close() {
	if s == nil || s.db == nil {
		return
	}
	_ = s.db.Close()
}

func (s *SQLiteStore) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

func (s *SQLiteStore) IndexProfile(ctx context.Context) (IndexProfile, error) {
	return readIndexProfile(ctx, s.db)
}

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

func (s *SQLiteStore) InsertChangeEvents(ctx context.Context, events []ChangeEvent) error {
	if len(events) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for _, ev := range events {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO change_events(
				run_id, page_id, event_type, old_version, new_version, old_parent, new_parent, old_title, new_title, summary_short, created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, ev.RunID, ev.PageID, ev.Type, zeroToNull(ev.OldVersion), zeroToNull(ev.NewVersion), nullIfEmpty(ev.OldParent), nullIfEmpty(ev.NewParent), nullIfEmpty(ev.OldTitle), nullIfEmpty(ev.NewTitle), ev.Summary, currentTimestamp()); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLiteStore) InsertPageChangeDiffs(ctx context.Context, diffs []PageChangeDiff) error {
	if len(diffs) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for _, d := range diffs {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO page_change_diffs(
				run_id, page_id, old_version, new_version, change_kind,
				title_changed, parent_changed, body_raw_changed, body_norm_changed,
				body_hash_old, body_hash_new, diagram_change_detected, diagram_content_unparsed,
				summary, excerpts_json, created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(run_id, page_id) DO UPDATE SET
				old_version = excluded.old_version,
				new_version = excluded.new_version,
				change_kind = excluded.change_kind,
				title_changed = excluded.title_changed,
				parent_changed = excluded.parent_changed,
				body_raw_changed = excluded.body_raw_changed,
				body_norm_changed = excluded.body_norm_changed,
				body_hash_old = excluded.body_hash_old,
				body_hash_new = excluded.body_hash_new,
				diagram_change_detected = excluded.diagram_change_detected,
				diagram_content_unparsed = excluded.diagram_content_unparsed,
				summary = excluded.summary,
				excerpts_json = excluded.excerpts_json,
				created_at = excluded.created_at
		`, d.RunID, d.PageID, zeroToNull(d.OldVersion), zeroToNull(d.NewVersion), d.ChangeKind, d.TitleChanged, d.ParentChanged, d.BodyRawChanged, d.BodyNormChanged, nullIfEmpty(d.BodyHashOld), nullIfEmpty(d.BodyHashNew), d.DiagramChangeDetected, d.DiagramContentUnparsed, d.Summary, toJSON(d.Excerpts), currentTimestamp()); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLiteStore) SaveDigest(ctx context.Context, date time.Time, markdown string, stats map[string]any) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO daily_digests(digest_date, generated_at, markdown, stats_json)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(digest_date) DO UPDATE SET
			generated_at = excluded.generated_at,
			markdown = excluded.markdown,
			stats_json = excluded.stats_json
	`, date.Format("2006-01-02"), currentTimestamp(), markdown, toJSON(stats))
	return err
}

func (s *SQLiteStore) GetDigest(ctx context.Context, date time.Time) (string, error) {
	var markdown string
	err := s.db.QueryRowContext(ctx, `SELECT markdown FROM daily_digests WHERE digest_date = ?`, date.Format("2006-01-02")).Scan(&markdown)
	if err != nil {
		return "", err
	}
	return markdown, nil
}

func (s *SQLiteStore) ListChangeEventsForDate(ctx context.Context, date time.Time) ([]ChangeEvent, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT run_id, page_id, event_type, COALESCE(old_version, 0), COALESCE(new_version, 0),
			COALESCE(old_parent, ''), COALESCE(new_parent, ''), COALESCE(old_title, ''), COALESCE(new_title, ''), summary_short
		FROM change_events
		WHERE substr(created_at, 1, 10) = ?
		ORDER BY created_at ASC, event_id ASC
	`, date.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]ChangeEvent, 0)
	for rows.Next() {
		var item ChangeEvent
		if err := rows.Scan(&item.RunID, &item.PageID, &item.Type, &item.OldVersion, &item.NewVersion, &item.OldParent, &item.NewParent, &item.OldTitle, &item.NewTitle, &item.Summary); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) ListPageChangeDiffs(ctx context.Context, query PageChangeDiffQuery) ([]PageChangeDiff, error) {
	limit := query.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	dateText := ""
	if query.Date != nil {
		dateText = query.Date.Format("2006-01-02")
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			d.run_id,
			d.page_id,
			COALESCE(p.title, '') AS title,
			COALESCE(p.parent_page_id, '') AS parent_page_id,
			COALESCE(d.old_version, 0) AS old_version,
			COALESCE(d.new_version, 0) AS new_version,
			d.change_kind,
			d.title_changed,
			d.parent_changed,
			d.body_raw_changed,
			d.body_norm_changed,
			COALESCE(d.body_hash_old, '') AS body_hash_old,
			COALESCE(d.body_hash_new, '') AS body_hash_new,
			d.diagram_change_detected,
			d.diagram_content_unparsed,
			d.summary,
			d.excerpts_json,
			d.created_at
		FROM page_change_diffs d
		LEFT JOIN pages p ON p.page_id = d.page_id
		WHERE (? = '' OR substr(d.created_at, 1, 10) = ?)
		  AND (? = 0 OR d.run_id = ?)
		  AND (? = '' OR COALESCE(p.parent_page_id, '') = ?)
		ORDER BY d.created_at DESC, d.run_id DESC, d.page_id ASC
		LIMIT ?
	`, dateText, dateText, query.RunID, query.RunID, query.ParentPageID, query.ParentPageID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]PageChangeDiff, 0)
	for rows.Next() {
		var (
			item       PageChangeDiff
			excerptsRaw string
			createdAt   string
		)
		if err := rows.Scan(
			&item.RunID,
			&item.PageID,
			&item.Title,
			&item.ParentPageID,
			&item.OldVersion,
			&item.NewVersion,
			&item.ChangeKind,
			&item.TitleChanged,
			&item.ParentChanged,
			&item.BodyRawChanged,
			&item.BodyNormChanged,
			&item.BodyHashOld,
			&item.BodyHashNew,
			&item.DiagramChangeDetected,
			&item.DiagramContentUnparsed,
			&item.Summary,
			&excerptsRaw,
			&createdAt,
		); err != nil {
			return nil, err
		}
		if excerptsRaw != "" {
			_ = json.Unmarshal([]byte(excerptsRaw), &item.Excerpts)
		}
		item.CreatedAt = mustParseTime(createdAt)
		out = append(out, item)
	}
	return out, rows.Err()
}

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

func sqliteDSN(path string) string {
	return fmt.Sprintf("file:%s?_busy_timeout=5000&_foreign_keys=on", path)
}

func configureSQLite(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `PRAGMA journal_mode=WAL;`); err != nil {
		return fmt.Errorf("enable wal: %w", err)
	}
	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys=ON;`); err != nil {
		return fmt.Errorf("enable foreign keys: %w", err)
	}
	return nil
}

func bootstrapSchema(ctx context.Context, db *sql.DB) error {
	schemaSQL, err := schemaFS.ReadFile("schema.sql")
	if err != nil {
		return fmt.Errorf("read embedded schema: %w", err)
	}
	if _, err := db.ExecContext(ctx, string(schemaSQL)); err != nil {
		return fmt.Errorf("apply sqlite schema: %w", err)
	}
	return nil
}

func ensureVectorTable(ctx context.Context, db *sql.DB, profile IndexProfile) error {
	var exists int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(1) FROM sqlite_master WHERE type = 'table' AND name = 'chunk_vectors'`).Scan(&exists); err != nil {
		return err
	}
	if exists > 0 {
		return nil
	}
	ddl := fmt.Sprintf(`CREATE VIRTUAL TABLE chunk_vectors USING vec0(embedding float[%d] distance_metric=cosine);`, profile.EmbeddingDimension)
	if _, err := db.ExecContext(ctx, ddl); err != nil {
		return fmt.Errorf("create vec0 table: %w", err)
	}
	return nil
}

func ensureIndexProfile(ctx context.Context, db *sql.DB, profile IndexProfile) error {
	current, err := readIndexProfile(ctx, db)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		_, err := db.ExecContext(ctx, `
			INSERT INTO replica_meta(
				singleton, schema_version, embedding_provider, embedding_model, embedding_dimension, chunking_version, embedding_normalization, created_at, updated_at
			) VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?)
		`, profile.SchemaVersion, profile.EmbeddingProvider, profile.EmbeddingModel, profile.EmbeddingDimension, profile.ChunkingVersion, profile.EmbeddingNormalization, currentTimestamp(), currentTimestamp())
		return err
	}
	if current != profile {
		return fmt.Errorf("reindex required: index profile mismatch (have %+v want %+v)", current, profile)
	}
	return nil
}

func readIndexProfile(ctx context.Context, db *sql.DB) (IndexProfile, error) {
	var profile IndexProfile
	err := db.QueryRowContext(ctx, `
		SELECT schema_version, embedding_provider, embedding_model, embedding_dimension, chunking_version, embedding_normalization
		FROM replica_meta
		WHERE singleton = 1
	`).Scan(&profile.SchemaVersion, &profile.EmbeddingProvider, &profile.EmbeddingModel, &profile.EmbeddingDimension, &profile.ChunkingVersion, &profile.EmbeddingNormalization)
	if err != nil {
		return IndexProfile{}, err
	}
	return profile, nil
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

func currentTimestamp() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func storeTime(t time.Time) string {
	if t.IsZero() {
		return currentTimestamp()
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func mustParseTime(raw string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02 15:04:05Z07:00"} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

