package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"confluence-replica/internal/confluence"
	"confluence-replica/internal/diff"
	"confluence-replica/internal/logx"
	"confluence-replica/internal/store"
)

type Service struct {
	client *confluence.Client
	store  store.Store
	emb    Embedder
}

type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

func NewService(client *confluence.Client, st store.Store, emb Embedder) *Service {
	return &Service{client: client, store: st, emb: emb}
}

func (s *Service) Bootstrap(ctx context.Context, parentID string) error {
	return s.syncInternal(ctx, parentID, "bootstrap")
}

func (s *Service) Sync(ctx context.Context, parentID string) error {
	return s.syncInternal(ctx, parentID, "sync")
}

func (s *Service) syncInternal(ctx context.Context, parentID, mode string) error {
	logx.Infof("[ingest] sync_start mode=%s parent_id=%s", mode, parentID)
	runID, err := s.store.BeginSyncRun(ctx, mode)
	if err != nil {
		return err
	}
	logx.Infof("[ingest] sync_run_id mode=%s run_id=%d", mode, runID)

	before, err := s.store.ListCurrentStates(ctx)
	if err != nil {
		_ = s.store.FinishSyncRun(ctx, runID, "failed", map[string]any{"error": err.Error()})
		return err
	}
	logx.Infof("[ingest] snapshot_before pages=%d", len(before))

	pages, err := s.client.WalkTree(ctx, parentID)
	if err != nil {
		_ = s.store.FinishSyncRun(ctx, runID, "failed", map[string]any{"error": err.Error()})
		return err
	}
	logx.Infof("[ingest] fetched_pages pages=%d", len(pages))

	after := make([]diff.PageState, 0, len(pages))
	for idx, p := range pages {
		bodyNorm := diff.NormalizeText(stripHTML(p.Body.Storage.Value))
		bodyHash := diff.HashNormalizedText(bodyNorm)
		parentID := ""
		if len(p.Ancestors) > 0 {
			parentID = p.Ancestors[len(p.Ancestors)-1].ID
		}

		up := store.Page{
			PageID:       p.ID,
			SpaceKey:     p.Space.Key,
			Title:        p.Title,
			ParentPageID: parentID,
			CurrentVer:   p.Version.Number,
			UpdatedAt:    parseConfluenceTime(p.Version.When),
			CreatedAt:    p.CreatedAt,
			PathHash:     hashPath(p.Ancestors),
			Tags:         labels(p.Metadata),
			Status:       p.Status,
		}
		v := store.PageVersion{
			PageID:     p.ID,
			Version:    p.Version.Number,
			AuthorID:   p.Version.By.AccountID,
			BodyRaw:    p.Body.Storage.Value,
			BodyNorm:   bodyNorm,
			BodyHash:   bodyHash,
			Title:      p.Title,
			ParentPage: parentID,
			FetchedAt:  time.Now().UTC(),
		}
		chunks := toChunks(p.ID, p.Version.Number, stripHTML(p.Body.Storage.Value))
		if err := s.fillChunkEmbeddings(ctx, chunks); err != nil {
			_ = s.store.FinishSyncRun(ctx, runID, "failed", map[string]any{"error": err.Error()})
			return err
		}
		logx.Debugf("[ingest] page_prepare index=%d/%d page_id=%s version=%d chunks=%d parent_id=%s", idx+1, len(pages), p.ID, p.Version.Number, len(chunks), parentID)

		if err := s.store.UpsertPageWithVersion(ctx, up, v, chunks); err != nil {
			_ = s.store.FinishSyncRun(ctx, runID, "failed", map[string]any{"error": err.Error()})
			return err
		}
		logx.Debugf("[ingest] page_saved page_id=%s version=%d", p.ID, p.Version.Number)
		after = append(after, diff.PageState{PageID: p.ID, Title: p.Title, ParentPageID: parentID, Version: p.Version.Number, BodyNormHash: bodyHash, Exists: true})
	}

	beforeStates := make([]diff.PageState, 0, len(before))
	for _, b := range before {
		beforeStates = append(beforeStates, diff.PageState{PageID: b.PageID, Title: b.Title, ParentPageID: b.ParentPageID, Version: b.Version, BodyNormHash: b.BodyNormHash, Exists: true})
	}
	events := diff.DetectChanges(beforeStates, after)
	persisted := make([]store.ChangeEvent, 0, len(events))
	for _, e := range events {
		persisted = append(persisted, store.ChangeEvent{
			RunID:      runID,
			PageID:     e.PageID,
			Type:       string(e.Type),
			OldVersion: e.OldVersion,
			NewVersion: e.NewVersion,
			OldParent:  e.OldParent,
			NewParent:  e.NewParent,
			OldTitle:   e.OldTitle,
			NewTitle:   e.NewTitle,
			Summary:    e.Summary,
		})
	}
	if err := s.store.InsertChangeEvents(ctx, persisted); err != nil {
		_ = s.store.FinishSyncRun(ctx, runID, "failed", map[string]any{"error": err.Error()})
		return err
	}
	diffRows, err := s.buildPageChangeDiffs(ctx, runID, events)
	if err != nil {
		_ = s.store.FinishSyncRun(ctx, runID, "failed", map[string]any{"error": err.Error()})
		return err
	}
	if err := s.store.InsertPageChangeDiffs(ctx, diffRows); err != nil {
		_ = s.store.FinishSyncRun(ctx, runID, "failed", map[string]any{"error": err.Error()})
		return err
	}
	logx.Infof("[ingest] changes_detected total=%d", len(events))

	if err := s.store.FinishSyncRun(ctx, runID, "success", map[string]any{"pages_scanned": len(pages), "events": len(events)}); err != nil {
		return err
	}
	logx.Infof("[ingest] sync_done mode=%s run_id=%d pages=%d events=%d", mode, runID, len(pages), len(events))
	return nil
}

func toChunks(pageID string, version int, text string) []store.Chunk {
	const chunkSize = 900
	if text == "" {
		return nil
	}
	runes := []rune(text)
	out := make([]store.Chunk, 0)
	for i, idx := 0, 0; i < len(runes); i, idx = i+chunkSize, idx+1 {
		end := i + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		chunkText := strings.TrimSpace(string(runes[i:end]))
		if chunkText == "" {
			continue
		}
		h := sha256.Sum256([]byte(chunkText))
		out = append(out, store.Chunk{
			PageID:     pageID,
			Version:    version,
			ChunkID:    fmt.Sprintf("%s:%d:%d", pageID, version, idx),
			ChunkText:  chunkText,
			ChunkHash:  hex.EncodeToString(h[:]),
			TokenCount: len(strings.Fields(chunkText)),
		})
	}
	return out
}

func labels(m confluence.Metadata) []string {
	out := make([]string, 0, len(m.Labels.Results))
	for _, l := range m.Labels.Results {
		out = append(out, l.Name)
	}
	return out
}

func hashPath(ancestors []confluence.Ancestor) string {
	if len(ancestors) == 0 {
		return ""
	}
	parts := make([]string, 0, len(ancestors))
	for _, a := range ancestors {
		parts = append(parts, a.ID)
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "/")))
	return hex.EncodeToString(sum[:])
}

func parseConfluenceTime(v string) time.Time {
	if v == "" {
		return time.Now().UTC()
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Now().UTC()
	}
	return t.UTC()
}

func stripHTML(in string) string {
	in = strings.ReplaceAll(in, "<br/>", "\n")
	in = strings.ReplaceAll(in, "<br>", "\n")
	in = strings.ReplaceAll(in, "</p>", "\n")
	out := strings.Builder{}
	inTag := false
	for _, r := range in {
		switch r {
		case '<':
			inTag = true
		case '>':
			inTag = false
		default:
			if !inTag {
				out.WriteRune(r)
			}
		}
	}
	return strings.TrimSpace(out.String())
}

func (s *Service) fillChunkEmbeddings(ctx context.Context, chunks []store.Chunk) error {
	if s.emb == nil {
		return nil
	}
	for i := range chunks {
		vec, err := s.emb.Embed(ctx, chunks[i].ChunkText)
		if err != nil {
			return fmt.Errorf("embed chunk %s: %w", chunks[i].ChunkID, err)
		}
		chunks[i].Embedding = vec
	}
	if len(chunks) > 0 {
		logx.Debugf("[ingest] embeddings_done chunks=%d", len(chunks))
	}
	return nil
}

func (s *Service) buildPageChangeDiffs(ctx context.Context, runID int64, events []diff.Event) ([]store.PageChangeDiff, error) {
	out := make([]store.PageChangeDiff, 0, len(events))
	for _, e := range events {
		row := store.PageChangeDiff{
			RunID:      runID,
			PageID:     e.PageID,
			OldVersion: e.OldVersion,
			NewVersion: e.NewVersion,
			ChangeKind: string(e.Type),
			Summary:    e.Summary,
		}

		oldDoc, hasOld, err := s.loadPageVersion(ctx, e.PageID, e.OldVersion)
		if err != nil {
			return nil, err
		}
		newDoc, hasNew, err := s.loadPageVersion(ctx, e.PageID, e.NewVersion)
		if err != nil {
			return nil, err
		}

		if hasNew {
			row.Title = newDoc.Title
			row.ParentPageID = newDoc.ParentPageID
		} else if hasOld {
			row.Title = oldDoc.Title
			row.ParentPageID = oldDoc.ParentPageID
		}

		row.TitleChanged = e.OldTitle != e.NewTitle
		row.ParentChanged = e.OldParent != e.NewParent

		oldRaw, newRaw := "", ""
		oldNorm, newNorm := "", ""
		if hasOld {
			oldRaw = oldDoc.BodyRaw
			oldNorm = oldDoc.BodyNorm
			row.BodyHashOld = oldDoc.BodyHash
		}
		if hasNew {
			newRaw = newDoc.BodyRaw
			newNorm = newDoc.BodyNorm
			row.BodyHashNew = newDoc.BodyHash
		}

		row.BodyRawChanged = oldRaw != newRaw
		row.BodyNormChanged = oldNorm != newNorm
		row.DiagramChangeDetected = detectDiagramMetaChange(oldRaw, newRaw)
		row.DiagramContentUnparsed = row.DiagramChangeDetected
		row.Excerpts = buildChangeExcerpt(row, oldRaw, newRaw, oldNorm, newNorm, hasOld, hasNew, e)

		out = append(out, row)
	}
	return out, nil
}

func (s *Service) loadPageVersion(ctx context.Context, pageID string, version int) (store.PageDocument, bool, error) {
	if version <= 0 {
		return store.PageDocument{}, false, nil
	}
	doc, err := s.store.GetPageVersion(ctx, pageID, version)
	if err != nil {
		return store.PageDocument{}, false, fmt.Errorf("load page %s version %d: %w", pageID, version, err)
	}
	return doc, true, nil
}

func detectDiagramMetaChange(oldRaw, newRaw string) bool {
	if oldRaw == newRaw {
		return false
	}
	return strings.Contains(oldRaw, `ac:name="drawio"`) || strings.Contains(newRaw, `ac:name="drawio"`)
}

func buildChangeExcerpt(row store.PageChangeDiff, oldRaw, newRaw, oldNorm, newNorm string, hasOld, hasNew bool, e diff.Event) store.ChangeExcerpts {
	switch e.Type {
	case diff.ChangeCreated:
		if hasNew {
			_, after := firstDifferenceExcerpt("", newNorm, 220)
			return store.ChangeExcerpts{After: after, Source: "body_norm"}
		}
	case diff.ChangeDeleted:
		if hasOld {
			before, _ := firstDifferenceExcerpt(oldNorm, "", 220)
			return store.ChangeExcerpts{Before: before, Source: "body_norm"}
		}
	}

	if row.BodyNormChanged {
		before, after := firstDifferenceExcerpt(oldNorm, newNorm, 220)
		return store.ChangeExcerpts{Before: before, After: after, Source: "body_norm"}
	}
	if row.BodyRawChanged {
		before, after := firstDifferenceExcerpt(oldRaw, newRaw, 220)
		return store.ChangeExcerpts{Before: before, After: after, Source: "body_raw"}
	}
	if row.TitleChanged || row.ParentChanged {
		before := strings.TrimSpace(fmt.Sprintf("title=%s parent=%s", e.OldTitle, e.OldParent))
		after := strings.TrimSpace(fmt.Sprintf("title=%s parent=%s", e.NewTitle, e.NewParent))
		return store.ChangeExcerpts{Before: before, After: after, Source: "metadata"}
	}
	return store.ChangeExcerpts{}
}

func firstDifferenceExcerpt(oldText, newText string, span int) (string, string) {
	if span <= 0 {
		span = 120
	}
	oldRunes := []rune(oldText)
	newRunes := []rune(newText)
	max := len(oldRunes)
	if len(newRunes) < max {
		max = len(newRunes)
	}
	pos := 0
	for pos < max && oldRunes[pos] == newRunes[pos] {
		pos++
	}
	return excerptWindow(oldRunes, pos, span), excerptWindow(newRunes, pos, span)
}

func excerptWindow(runes []rune, pos int, span int) string {
	if len(runes) == 0 {
		return ""
	}
	if pos < 0 {
		pos = 0
	}
	if pos > len(runes) {
		pos = len(runes)
	}
	start := pos - span/2
	if start < 0 {
		start = 0
	}
	end := start + span
	if end > len(runes) {
		end = len(runes)
		start = end - span
		if start < 0 {
			start = 0
		}
	}
	return strings.TrimSpace(string(runes[start:end]))
}
