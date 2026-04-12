package search

import (
	"sort"

	"confluence-replica/internal/store"
)

const (
	defaultRRFK            = 60
	defaultCandidateWindow = 50
)

type Hit struct {
	PageID           string  `json:"page_id"`
	ChunkID          string  `json:"chunk_id,omitempty"`
	Title            string  `json:"title"`
	Snippet          string  `json:"snippet"`
	Version          int     `json:"version"`
	LexicalRank      int     `json:"lexical_rank,omitempty"`
	LexicalRankValue float64 `json:"lexical_rank_value,omitempty"`
	VectorRank       int     `json:"vector_rank,omitempty"`
	VectorDistance   float64 `json:"vector_distance,omitempty"`
	FusionScore      float64 `json:"fusion_score"`
}

func DefaultCandidateWindow() int {
	return defaultCandidateWindow
}

func Fuse(lexical []store.LexicalSearchRow, semantic []store.SemanticSearchRow, limit int) []Hit {
	if limit <= 0 {
		limit = len(lexical) + len(semantic)
	}

	byChunk := make(map[string]*Hit, len(lexical)+len(semantic))
	for _, row := range lexical {
		hit := ensureHit(byChunk, row.ChunkID)
		hit.PageID = row.PageID
		hit.ChunkID = row.ChunkID
		hit.Version = row.Version
		hit.Title = row.Title
		if row.Snippet != "" {
			hit.Snippet = row.Snippet
		}
		hit.LexicalRank = row.Rank
		hit.LexicalRankValue = row.RankValue
		hit.FusionScore += reciprocalRankScore(row.Rank)
	}
	for _, row := range semantic {
		hit := ensureHit(byChunk, row.ChunkID)
		hit.PageID = row.PageID
		hit.ChunkID = row.ChunkID
		hit.Version = row.Version
		hit.Title = row.Title
		if hit.Snippet == "" && row.Snippet != "" {
			hit.Snippet = row.Snippet
		}
		hit.VectorRank = row.Rank
		hit.VectorDistance = row.Distance
		hit.FusionScore += reciprocalRankScore(row.Rank)
	}

	out := make([]Hit, 0, len(byChunk))
	for _, hit := range byChunk {
		out = append(out, *hit)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].FusionScore == out[j].FusionScore {
			if out[i].PageID == out[j].PageID {
				return out[i].ChunkID < out[j].ChunkID
			}
			return out[i].PageID < out[j].PageID
		}
		return out[i].FusionScore > out[j].FusionScore
	})
	if limit > len(out) {
		limit = len(out)
	}
	return out[:limit]
}

func ensureHit(byChunk map[string]*Hit, chunkID string) *Hit {
	if hit, ok := byChunk[chunkID]; ok {
		return hit
	}
	hit := &Hit{}
	byChunk[chunkID] = hit
	return hit
}

func reciprocalRankScore(rank int) float64 {
	if rank <= 0 {
		return 0
	}
	return 1.0 / float64(defaultRRFK+rank)
}
