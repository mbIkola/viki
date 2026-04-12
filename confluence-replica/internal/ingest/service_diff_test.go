package ingest

import (
	"testing"

	"confluence-replica/internal/diff"
	"confluence-replica/internal/store"
)

func TestDetectDiagramMetaChange(t *testing.T) {
	oldRaw := `<p>Text</p><ac:structured-macro ac:name="drawio"><ac:parameter ac:name="revision">6</ac:parameter></ac:structured-macro>`
	newRaw := `<p>Text</p><ac:structured-macro ac:name="drawio"><ac:parameter ac:name="revision">8</ac:parameter></ac:structured-macro>`
	if !detectDiagramMetaChange(oldRaw, newRaw) {
		t.Fatalf("expected diagram metadata change to be detected")
	}
}

func TestBuildChangeExcerptBodyNorm(t *testing.T) {
	row := store.PageChangeDiff{BodyNormChanged: true}
	ex := buildChangeExcerpt(row, "", "", "before payload", "after payload", true, true, diff.Event{Type: diff.ChangeUpdated})
	if ex.Source != "body_norm" {
		t.Fatalf("expected body_norm source, got %q", ex.Source)
	}
	if ex.Before == "" || ex.After == "" {
		t.Fatalf("expected before/after excerpts")
	}
}

func TestBuildChangeExcerptBodyRawFallback(t *testing.T) {
	row := store.PageChangeDiff{BodyRawChanged: true, BodyNormChanged: false}
	ex := buildChangeExcerpt(row, "revision=6", "revision=8", "same", "same", true, true, diff.Event{Type: diff.ChangeUpdated})
	if ex.Source != "body_raw" {
		t.Fatalf("expected body_raw source, got %q", ex.Source)
	}
	if ex.Before == "" || ex.After == "" {
		t.Fatalf("expected raw before/after excerpts")
	}
}

func TestApplyScopePartialSuppressesDeleted(t *testing.T) {
	events := []diff.Event{
		{PageID: "1", Type: diff.ChangeCreated},
		{PageID: "2", Type: diff.ChangeDeleted},
		{PageID: "3", Type: diff.ChangeUpdated},
	}
	filtered, suppressed := applyScope(events, ScopeModePartial)
	if suppressed != 1 {
		t.Fatalf("expected one suppressed delete, got %d", suppressed)
	}
	if len(filtered) != 2 {
		t.Fatalf("expected 2 events after partial scope filtering, got %d", len(filtered))
	}
	for _, ev := range filtered {
		if ev.Type == diff.ChangeDeleted {
			t.Fatalf("deleted event should not survive in partial mode")
		}
	}
}

func TestApplyScopeFullKeepsDeleted(t *testing.T) {
	events := []diff.Event{
		{PageID: "1", Type: diff.ChangeDeleted},
	}
	filtered, suppressed := applyScope(events, ScopeModeFull)
	if suppressed != 0 {
		t.Fatalf("expected no suppression in full mode, got %d", suppressed)
	}
	if len(filtered) != 1 || filtered[0].Type != diff.ChangeDeleted {
		t.Fatalf("expected deleted event to remain in full mode")
	}
}
