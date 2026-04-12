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

type SearchStore interface {
	SearchLexical(ctx context.Context, query string, limit int) ([]store.LexicalSearchRow, error)
	SearchSemantic(ctx context.Context, embedding []float32, limit int) ([]store.SemanticSearchRow, error)
}

type Service struct {
	store    SearchStore
	embedder Embedder
}

func NewService(st SearchStore, emb Embedder) *Service {
	if emb == nil {
		emb = NoopEmbedder{}
	}
	return &Service{store: st, embedder: emb}
}

func (s *Service) Query(ctx context.Context, query string, limit int) ([]Hit, error) {
	emb, err := s.embedder.Embed(ctx, query)
	if err != nil {
		return nil, err
	}
	lexicalRows, err := s.store.SearchLexical(ctx, query, DefaultCandidateWindow())
	if err != nil {
		return nil, err
	}
	var semanticRows []store.SemanticSearchRow
	if len(emb) > 0 {
		semanticRows, err = s.store.SearchSemantic(ctx, emb, DefaultCandidateWindow())
		if err != nil {
			return nil, err
		}
	}
	return Fuse(lexicalRows, semanticRows, limit), nil
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
			ChunkID: h.ChunkID,
			Title:   h.Title,
			Snippet: h.Snippet,
			Score:   h.FusionScore,
		})
	}
	return out, nil
}
