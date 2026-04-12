package search

import (
	"math"
	"testing"

	"confluence-replica/internal/store"
)

func TestFuseUsesReciprocalRankAcrossSources(t *testing.T) {
	lexical := []store.LexicalSearchRow{
		{
			PageID:    "page-1",
			ChunkID:   "page-1:7:0",
			Version:   7,
			Title:     "Runbook",
			Snippet:   "lexical snippet",
			Rank:      1,
			RankValue: 12.5,
		},
	}
	semantic := []store.SemanticSearchRow{
		{
			PageID:         "page-2",
			ChunkID:        "page-2:4:0",
			Version:        4,
			Title:          "Guide",
			Snippet:        "semantic snippet",
			Rank:           1,
			Distance:       0.12,
			EmbeddingModel: "nomic-embed-text",
		},
		{
			PageID:         "page-1",
			ChunkID:        "page-1:7:0",
			Version:        7,
			Title:          "Runbook",
			Snippet:        "semantic fallback",
			Rank:           3,
			Distance:       0.08,
			EmbeddingModel: "nomic-embed-text",
		},
	}

	hits := Fuse(lexical, semantic, 10)
	if len(hits) != 2 {
		t.Fatalf("expected 2 fused hits, got %d", len(hits))
	}
	if hits[0].ChunkID != "page-1:7:0" {
		t.Fatalf("expected overlapping chunk first, got %#v", hits[0])
	}
	if hits[0].LexicalRank != 1 || hits[0].VectorRank != 3 {
		t.Fatalf("expected preserved ranks, got %#v", hits[0])
	}
	if hits[0].LexicalRankValue != 12.5 {
		t.Fatalf("expected lexical rank value to survive fusion, got %#v", hits[0])
	}
	if hits[0].VectorDistance != 0.08 {
		t.Fatalf("expected vector distance to survive fusion, got %#v", hits[0])
	}
	if hits[0].Snippet != "lexical snippet" {
		t.Fatalf("expected lexical snippet to win when present, got %#v", hits[0])
	}

	want := (1.0 / 61.0) + (1.0 / 63.0)
	if math.Abs(hits[0].FusionScore-want) > 1e-9 {
		t.Fatalf("unexpected fusion score: got %.12f want %.12f", hits[0].FusionScore, want)
	}
}

func TestFuseBreaksTiesDeterministically(t *testing.T) {
	lexical := []store.LexicalSearchRow{
		{
			PageID:    "page-b",
			ChunkID:   "chunk-b",
			Version:   1,
			Title:     "B",
			Snippet:   "b",
			Rank:      1,
			RankValue: 10,
		},
	}
	semantic := []store.SemanticSearchRow{
		{
			PageID:         "page-a",
			ChunkID:        "chunk-a",
			Version:        1,
			Title:          "A",
			Snippet:        "a",
			Rank:           1,
			Distance:       0.3,
			EmbeddingModel: "nomic-embed-text",
		},
	}

	hits := Fuse(lexical, semantic, 10)
	if len(hits) != 2 {
		t.Fatalf("expected 2 fused hits, got %d", len(hits))
	}
	if hits[0].PageID != "page-a" {
		t.Fatalf("expected deterministic page-id tie break, got order %#v", hits)
	}
}
