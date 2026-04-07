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
