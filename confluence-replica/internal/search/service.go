package search

import (
	"context"

	"confluence-replica/internal/rag"
	"confluence-replica/internal/store"
)

type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

type NoopEmbedder struct{}

func (n NoopEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return nil, nil
}

type Service struct {
	store    store.Store
	embedder Embedder
	weights  Weights
}

func NewService(st store.Store, emb Embedder) *Service {
	if emb == nil {
		emb = NoopEmbedder{}
	}
	return &Service{store: st, embedder: emb, weights: DefaultWeights()}
}

func (s *Service) Query(ctx context.Context, query string, limit int) ([]Hit, error) {
	emb, err := s.embedder.Embed(ctx, query)
	if err != nil {
		return nil, err
	}
	rows, err := s.store.SearchHybrid(ctx, query, emb, limit)
	if err != nil {
		return nil, err
	}
	hits := make([]Hit, 0, len(rows))
	for _, r := range rows {
		hits = append(hits, Hit{
			PageID:         r.PageID,
			Version:        r.Version,
			Title:          r.Title,
			Snippet:        r.Snippet,
			FTSScore:       r.FTSScore,
			SemanticScore:  r.SemanticScore,
			Freshness:      r.Freshness,
			ParentDistance: r.ParentDistance,
			VersionRecency: r.VersionRecency,
			TagMatch:       r.TagMatch,
		})
	}
	return Rerank(hits, s.weights), nil
}

func (s *Service) Retrieve(ctx context.Context, query string, k int) ([]rag.SearchHit, error) {
	hits, err := s.Query(ctx, query, k)
	if err != nil {
		return nil, err
	}
	out := make([]rag.SearchHit, 0, len(hits))
	for _, h := range hits {
		out = append(out, rag.SearchHit{
			PageID:  h.PageID,
			Version: h.Version,
			Title:   h.Title,
			Snippet: h.Snippet,
			Score:   h.FinalScore,
		})
	}
	return out, nil
}
