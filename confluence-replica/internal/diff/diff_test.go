package diff

import "testing"

func TestDetectChanges(t *testing.T) {
	oldPages := []PageState{
		{PageID: "1", Title: "A", ParentPageID: "10", Version: 1, BodyNormHash: "h1", Exists: true},
		{PageID: "2", Title: "B", ParentPageID: "10", Version: 1, BodyNormHash: "h2", Exists: true},
		{PageID: "3", Title: "C", ParentPageID: "11", Version: 2, BodyNormHash: "h3", Exists: true},
	}
	newPages := []PageState{
		{PageID: "1", Title: "A", ParentPageID: "10", Version: 1, BodyNormHash: "h1", Exists: true},
		{PageID: "2", Title: "B2", ParentPageID: "12", Version: 2, BodyNormHash: "h2-new", Exists: true},
		{PageID: "4", Title: "D", ParentPageID: "10", Version: 1, BodyNormHash: "h4", Exists: true},
	}

	events := DetectChanges(oldPages, newPages)
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	seen := map[ChangeType]bool{}
	for _, ev := range events {
		seen[ev.Type] = true
	}

	if !seen[ChangeMoved] {
		t.Fatalf("expected moved event")
	}
	if !seen[ChangeCreated] {
		t.Fatalf("expected created event")
	}
	if !seen[ChangeDeleted] {
		t.Fatalf("expected deleted event")
	}
}

func TestNormalizeTextAvoidsFormatNoise(t *testing.T) {
	a := "Hello   World\n\n  Next line"
	b := "Hello World\nNext line"
	if NormalizeText(a) != NormalizeText(b) {
		t.Fatalf("expected normalized text to match")
	}
}
