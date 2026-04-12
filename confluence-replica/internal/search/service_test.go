package search

import (
	"context"
	"testing"

	"confluence-replica/internal/store"
)

type fakeSearchStore struct {
	lexicalRows   []store.LexicalSearchRow
	semanticRows  []store.SemanticSearchRow
	lexicalLimit  int
	semanticLimit int
	semanticCalls int
}

func (f *fakeSearchStore) SearchLexical(_ context.Context, _ string, limit int) ([]store.LexicalSearchRow, error) {
	f.lexicalLimit = limit
	return f.lexicalRows, nil
}

func (f *fakeSearchStore) SearchSemantic(_ context.Context, _ []float32, limit int) ([]store.SemanticSearchRow, error) {
	f.semanticCalls++
	f.semanticLimit = limit
	return f.semanticRows, nil
}

type staticEmbedder struct {
	embedding []float32
}

func (s staticEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return s.embedding, nil
}

func TestServiceQueryUsesCandidateWindowAndFusion(t *testing.T) {
	st := &fakeSearchStore{
		lexicalRows: []store.LexicalSearchRow{
			{
				PageID:    "page-1",
				ChunkID:   "shared",
				Version:   7,
				Title:     "Runbook",
				Snippet:   "lexical snippet",
				Rank:      1,
				RankValue: 11,
			},
		},
		semanticRows: []store.SemanticSearchRow{
			{
				PageID:         "page-1",
				ChunkID:        "shared",
				Version:        7,
				Title:          "Runbook",
				Snippet:        "semantic snippet",
				Rank:           2,
				Distance:       0.05,
				EmbeddingModel: "nomic-embed-text",
			},
		},
	}

	svc := NewService(st, staticEmbedder{embedding: []float32{0.1, 0.2}})
	hits, err := svc.Query(context.Background(), "incident", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if st.lexicalLimit != DefaultCandidateWindow() {
		t.Fatalf("expected lexical candidate window %d, got %d", DefaultCandidateWindow(), st.lexicalLimit)
	}
	if st.semanticLimit != DefaultCandidateWindow() {
		t.Fatalf("expected semantic candidate window %d, got %d", DefaultCandidateWindow(), st.semanticLimit)
	}
	if len(hits) != 1 {
		t.Fatalf("expected one fused hit, got %d", len(hits))
	}
	if hits[0].FusionScore <= 0 {
		t.Fatalf("expected positive fusion score, got %#v", hits[0])
	}
	if hits[0].Snippet != "lexical snippet" {
		t.Fatalf("expected lexical snippet to be preserved, got %#v", hits[0])
	}
}

func TestServiceQuerySkipsSemanticWhenEmbedderReturnsEmptyVector(t *testing.T) {
	st := &fakeSearchStore{
		lexicalRows: []store.LexicalSearchRow{
			{
				PageID:    "page-1",
				ChunkID:   "chunk-1",
				Version:   1,
				Title:     "Only lexical",
				Snippet:   "lexical only",
				Rank:      1,
				RankValue: 9,
			},
		},
	}

	svc := NewService(st, staticEmbedder{})
	hits, err := svc.Query(context.Background(), "lexical", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if st.semanticCalls != 0 {
		t.Fatalf("expected semantic search to be skipped, got %d calls", st.semanticCalls)
	}
	if len(hits) != 1 || hits[0].ChunkID != "chunk-1" {
		t.Fatalf("unexpected hits: %#v", hits)
	}
}
