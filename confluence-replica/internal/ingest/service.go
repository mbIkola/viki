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
