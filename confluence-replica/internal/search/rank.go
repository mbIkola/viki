package search

import "sort"

type Hit struct {
	PageID         string  `json:"page_id"`
	Title          string  `json:"title"`
	Snippet        string  `json:"snippet"`
	Version        int     `json:"version"`
	FTSScore       float64 `json:"fts_score"`
	SemanticScore  float64 `json:"semantic_score"`
	Freshness      float64 `json:"freshness"`
	ParentDistance float64 `json:"parent_distance"`
	VersionRecency float64 `json:"version_recency"`
	TagMatch       float64 `json:"tag_match"`
	FinalScore     float64 `json:"final_score"`
}

type Weights struct {
	FTS            float64
	Semantic       float64
	Freshness      float64
	ParentDistance float64
	VersionRecency float64
	TagMatch       float64
}

func DefaultWeights() Weights {
	return Weights{
		FTS:            0.30,
		Semantic:       0.30,
		Freshness:      0.15,
		ParentDistance: 0.10,
		VersionRecency: 0.10,
		TagMatch:       0.05,
	}
}

func Rerank(hits []Hit, w Weights) []Hit {
	out := make([]Hit, len(hits))
	copy(out, hits)
	for i := range out {
		distanceBoost := 1.0 / (1.0 + out[i].ParentDistance)
		out[i].FinalScore =
			w.FTS*out[i].FTSScore +
				w.Semantic*out[i].SemanticScore +
				w.Freshness*out[i].Freshness +
				w.ParentDistance*distanceBoost +
				w.VersionRecency*out[i].VersionRecency +
				w.TagMatch*out[i].TagMatch
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].FinalScore == out[j].FinalScore {
			return out[i].PageID < out[j].PageID
		}
		return out[i].FinalScore > out[j].FinalScore
	})
	return out
}
