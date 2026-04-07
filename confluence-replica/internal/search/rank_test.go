package search

import "testing"

func TestRerankBoostsFreshAndNearParent(t *testing.T) {
	hits := []Hit{
		{PageID: "1", FTSScore: 0.4, SemanticScore: 0.7, Freshness: 0.2, ParentDistance: 4, VersionRecency: 0.5, TagMatch: 0.1},
		{PageID: "2", FTSScore: 0.4, SemanticScore: 0.7, Freshness: 0.9, ParentDistance: 1, VersionRecency: 0.9, TagMatch: 0.2},
	}
	weights := DefaultWeights()
	ranked := Rerank(hits, weights)
	if len(ranked) != 2 {
		t.Fatalf("expected 2 hits")
	}
	if ranked[0].PageID != "2" {
		t.Fatalf("expected page 2 first, got %s", ranked[0].PageID)
	}
}
