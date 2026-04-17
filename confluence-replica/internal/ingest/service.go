package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
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

const (
	ScopeModeFull    = "full"
	ScopeModePartial = "partial"
)

func NewService(client *confluence.Client, st store.Store, emb Embedder) *Service {
	return &Service{client: client, store: st, emb: emb}
}

func (s *Service) Bootstrap(ctx context.Context, parentIDs []string, scopeMode string) error {
	return s.syncInternal(ctx, parentIDs, "bootstrap", scopeMode)
}

func (s *Service) Sync(ctx context.Context, parentIDs []string, scopeMode string) error {
	return s.syncInternal(ctx, parentIDs, "sync", scopeMode)
}

func (s *Service) syncInternal(ctx context.Context, parentIDs []string, mode, scopeMode string) error {
	parentIDs = normalizeIDs(parentIDs)
	if len(parentIDs) == 0 {
		return fmt.Errorf("at least one parent_id is required")
	}
	if scopeMode != ScopeModeFull && scopeMode != ScopeModePartial {
		return fmt.Errorf("invalid scope mode %q", scopeMode)
	}
	logx.Infof("[ingest] sync_start mode=%s scope_mode=%s parent_ids=%v", mode, scopeMode, parentIDs)
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

	rawFetched := 0
	pageByID := make(map[string]confluence.Page, 1024)
	for _, rootID := range parentIDs {
		pages, walkErr := s.client.WalkTree(ctx, rootID)
		if walkErr != nil {
			_ = s.store.FinishSyncRun(ctx, runID, "failed", map[string]any{
				"error":      walkErr.Error(),
				"parent_ids": parentIDs,
				"scope_mode": scopeMode,
			})
			return walkErr
		}
		rawFetched += len(pages)
		for _, p := range pages {
			existing, ok := pageByID[p.ID]
			if !ok || p.Version.Number >= existing.Version.Number {
				pageByID[p.ID] = p
			}
		}
	}
	pages := make([]confluence.Page, 0, len(pageByID))
	for _, p := range pageByID {
		pages = append(pages, p)
	}
	sort.Slice(pages, func(i, j int) bool { return pages[i].ID < pages[j].ID })
	logx.Infof("[ingest] fetched_pages raw=%d unique=%d", rawFetched, len(pages))

	after := make([]diff.PageState, 0, len(pages))
	for idx, p := range pages {
		state, err := s.UpsertConfluencePage(ctx, p)
		if err != nil {
			_ = s.store.FinishSyncRun(ctx, runID, "failed", map[string]any{"error": err.Error()})
			return err
		}
		logx.Debugf("[ingest] page_saved index=%d/%d page_id=%s version=%d parent_id=%s", idx+1, len(pages), state.PageID, state.Version, state.ParentPageID)
		after = append(after, state)
	}

	beforeStates := make([]diff.PageState, 0, len(before))
	for _, b := range before {
		beforeStates = append(beforeStates, diff.PageState{PageID: b.PageID, Title: b.Title, ParentPageID: b.ParentPageID, Version: b.Version, BodyNormHash: b.BodyNormHash, Exists: true})
	}
	rawEvents := diff.DetectChanges(beforeStates, after)
	events, deletedSuppressed := applyScope(rawEvents, scopeMode)
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

	if err := s.store.FinishSyncRun(ctx, runID, "success", map[string]any{
		"pages_scanned":      len(pages),
		"events":             len(events),
		"events_raw":         len(rawEvents),
		"deleted_suppressed": deletedSuppressed,
		"scope_mode":         scopeMode,
		"parent_ids":         parentIDs,
	}); err != nil {
		return err
	}
	logx.Infof("[ingest] sync_done mode=%s scope_mode=%s run_id=%d pages=%d events=%d", mode, scopeMode, runID, len(pages), len(events))
	return nil
}

func pageToStoreRecords(p confluence.Page, fetchedAt time.Time) (store.Page, store.PageVersion, []store.Chunk, diff.PageState) {
	bodyText := stripHTML(p.Body.Storage.Value)
	bodyNorm := diff.NormalizeText(bodyText)
	bodyHash := diff.HashNormalizedText(bodyNorm)
	pageParentID := ""
	if len(p.Ancestors) > 0 {
		pageParentID = p.Ancestors[len(p.Ancestors)-1].ID
	}

	up := store.Page{
		PageID:       p.ID,
		SpaceKey:     p.Space.Key,
		Title:        p.Title,
		ParentPageID: pageParentID,
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
		ParentPage: pageParentID,
		FetchedAt:  fetchedAt.UTC(),
	}
	chunks := toChunks(p.ID, p.Version.Number, bodyText)
	state := diff.PageState{
		PageID:       p.ID,
		Title:        p.Title,
		ParentPageID: pageParentID,
		Version:      p.Version.Number,
		BodyNormHash: bodyHash,
		Exists:       true,
	}
	return up, v, chunks, state
}

func (s *Service) UpsertConfluencePage(ctx context.Context, p confluence.Page) (diff.PageState, error) {
	up, v, chunks, state := pageToStoreRecords(p, time.Now().UTC())
	if err := s.fillChunkEmbeddings(ctx, chunks); err != nil {
		return diff.PageState{}, err
	}
	logx.Debugf("[ingest] page_prepare page_id=%s version=%d chunks=%d parent_id=%s", state.PageID, state.Version, len(chunks), state.ParentPageID)
	if err := s.store.UpsertPageWithVersion(ctx, up, v, chunks); err != nil {
		return diff.PageState{}, err
	}
	return state, nil
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

func normalizeIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, raw := range ids {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func applyScope(events []diff.Event, scopeMode string) ([]diff.Event, int) {
	if scopeMode != ScopeModePartial {
		return events, 0
	}
	filtered := make([]diff.Event, 0, len(events))
	suppressed := 0
	for _, e := range events {
		if e.Type == diff.ChangeDeleted {
			suppressed++
			continue
		}
		filtered = append(filtered, e)
	}
	return filtered, suppressed
}
