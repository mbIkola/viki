package store

import (
	"context"
	"encoding/json"
	"time"
)

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
			item        PageChangeDiff
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
