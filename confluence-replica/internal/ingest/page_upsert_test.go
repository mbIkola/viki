package ingest

import (
	"context"
	"strings"
	"testing"
	"time"

	"confluence-replica/internal/confluence"
	"confluence-replica/internal/diff"
	"confluence-replica/internal/store"
)

type captureEmbedder struct {
	calls int
	texts []string
	err   error
}

func (e *captureEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	if e.err != nil {
		return nil, e.err
	}
	e.calls++
	e.texts = append(e.texts, text)
	return []float32{float32(len(text))}, nil
}

type captureStore struct {
	store.Store

	upsertCalls int
	upsertErr   error
	lastPage    store.Page
	lastVersion store.PageVersion
	lastChunks  []store.Chunk
}

func (s *captureStore) UpsertPageWithVersion(_ context.Context, p store.Page, v store.PageVersion, chunks []store.Chunk) error {
	s.upsertCalls++
	s.lastPage = p
	s.lastVersion = v
	s.lastChunks = append([]store.Chunk(nil), chunks...)
	return s.upsertErr
}

func TestPageToStoreRecords(t *testing.T) {
	p := confluence.Page{
		ID:        "12345",
		Title:     "Runbook",
		Status:    "current",
		Space:     confluence.Space{Key: "OPS"},
		Ancestors: []confluence.Ancestor{{ID: "root"}, {ID: "parent"}},
		CreatedAt: time.Date(2025, time.March, 2, 12, 30, 0, 0, time.UTC),
	}
	p.Version.Number = 7
	p.Version.When = "2025-03-03T10:45:00Z"
	p.Version.By.AccountID = "author-1"
	p.Body.Storage.Value = "<p>Hello</p><p>World</p>"
	p.Metadata.Labels.Results = []struct {
		Name string `json:"name"`
	}{
		{Name: "alpha"},
		{Name: "beta"},
	}
	fetchedAt := time.Date(2025, time.March, 4, 14, 0, 0, 0, time.UTC)

	pageRec, versionRec, chunks, state := pageToStoreRecords(p, fetchedAt)

	expectedBodyText := stripHTML(p.Body.Storage.Value)
	expectedNorm := diff.NormalizeText(expectedBodyText)
	expectedHash := diff.HashNormalizedText(expectedNorm)

	if pageRec.PageID != p.ID {
		t.Fatalf("page id mismatch: got %q want %q", pageRec.PageID, p.ID)
	}
	if pageRec.ParentPageID != "parent" {
		t.Fatalf("parent id mismatch: got %q want %q", pageRec.ParentPageID, "parent")
	}
	if pageRec.CurrentVer != p.Version.Number {
		t.Fatalf("page version mismatch: got %d want %d", pageRec.CurrentVer, p.Version.Number)
	}
	if !pageRec.UpdatedAt.Equal(parseConfluenceTime(p.Version.When)) {
		t.Fatalf("updated at mismatch: got %v want %v", pageRec.UpdatedAt, parseConfluenceTime(p.Version.When))
	}
	if !pageRec.CreatedAt.Equal(p.CreatedAt) {
		t.Fatalf("created at mismatch: got %v want %v", pageRec.CreatedAt, p.CreatedAt)
	}
	if pageRec.PathHash != hashPath(p.Ancestors) {
		t.Fatalf("path hash mismatch: got %q want %q", pageRec.PathHash, hashPath(p.Ancestors))
	}
	if len(pageRec.Tags) != 2 || pageRec.Tags[0] != "alpha" || pageRec.Tags[1] != "beta" {
		t.Fatalf("unexpected labels: %#v", pageRec.Tags)
	}

	if versionRec.PageID != p.ID {
		t.Fatalf("version page id mismatch: got %q want %q", versionRec.PageID, p.ID)
	}
	if versionRec.Version != p.Version.Number {
		t.Fatalf("version number mismatch: got %d want %d", versionRec.Version, p.Version.Number)
	}
	if versionRec.AuthorID != "author-1" {
		t.Fatalf("author id mismatch: got %q want %q", versionRec.AuthorID, "author-1")
	}
	if versionRec.ParentPage != "parent" {
		t.Fatalf("version parent mismatch: got %q want %q", versionRec.ParentPage, "parent")
	}
	if versionRec.BodyNorm != expectedNorm {
		t.Fatalf("body norm mismatch: got %q want %q", versionRec.BodyNorm, expectedNorm)
	}
	if versionRec.BodyHash != expectedHash {
		t.Fatalf("body hash mismatch: got %q want %q", versionRec.BodyHash, expectedHash)
	}
	if !versionRec.FetchedAt.Equal(fetchedAt) {
		t.Fatalf("fetched at mismatch: got %v want %v", versionRec.FetchedAt, fetchedAt)
	}

	if len(chunks) != 1 {
		t.Fatalf("chunk count mismatch: got %d want 1", len(chunks))
	}
	if chunks[0].PageID != p.ID || chunks[0].Version != p.Version.Number {
		t.Fatalf("chunk identity mismatch: %#v", chunks[0])
	}
	if chunks[0].ChunkID != "12345:7:0" {
		t.Fatalf("chunk id mismatch: got %q want %q", chunks[0].ChunkID, "12345:7:0")
	}
	if chunks[0].ChunkText != expectedBodyText {
		t.Fatalf("chunk text mismatch: got %q want %q", chunks[0].ChunkText, expectedBodyText)
	}
	if chunks[0].TokenCount != len(strings.Fields(expectedBodyText)) {
		t.Fatalf("token count mismatch: got %d want %d", chunks[0].TokenCount, len(strings.Fields(expectedBodyText)))
	}
	if chunks[0].ChunkHash == "" {
		t.Fatalf("expected chunk hash to be set")
	}

	if state.PageID != p.ID {
		t.Fatalf("state page id mismatch: got %q want %q", state.PageID, p.ID)
	}
	if state.ParentPageID != "parent" {
		t.Fatalf("state parent mismatch: got %q want %q", state.ParentPageID, "parent")
	}
	if state.Version != p.Version.Number {
		t.Fatalf("state version mismatch: got %d want %d", state.Version, p.Version.Number)
	}
	if state.BodyNormHash != expectedHash {
		t.Fatalf("state body hash mismatch: got %q want %q", state.BodyNormHash, expectedHash)
	}
	if !state.Exists {
		t.Fatalf("state should be marked existing")
	}
}

func TestUpsertConfluencePage(t *testing.T) {
	embedder := &captureEmbedder{}
	st := &captureStore{}
	svc := &Service{
		store: st,
		emb:   embedder,
	}

	p := confluence.Page{
		ID:        "p-1",
		Title:     "Large Body",
		Status:    "current",
		Space:     confluence.Space{Key: "ENG"},
		Ancestors: []confluence.Ancestor{{ID: "root-1"}, {ID: "parent-1"}},
		CreatedAt: time.Date(2024, time.June, 15, 8, 0, 0, 0, time.UTC),
	}
	p.Version.Number = 3
	p.Version.When = "2024-06-16T09:10:11Z"
	p.Version.By.AccountID = "writer-2"
	p.Body.Storage.Value = "<p>" + strings.Repeat("word ", 600) + "</p>"

	state, err := svc.UpsertConfluencePage(context.Background(), p)
	if err != nil {
		t.Fatalf("unexpected upsert error: %v", err)
	}

	if st.upsertCalls != 1 {
		t.Fatalf("upsert calls mismatch: got %d want 1", st.upsertCalls)
	}
	if st.lastPage.PageID != p.ID || st.lastPage.CurrentVer != p.Version.Number {
		t.Fatalf("unexpected stored page record: %#v", st.lastPage)
	}
	if st.lastVersion.PageID != p.ID || st.lastVersion.Version != p.Version.Number {
		t.Fatalf("unexpected stored version record: %#v", st.lastVersion)
	}
	if len(st.lastChunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(st.lastChunks))
	}

	if embedder.calls != len(st.lastChunks) {
		t.Fatalf("embedder calls mismatch: got %d want %d", embedder.calls, len(st.lastChunks))
	}
	for _, c := range st.lastChunks {
		if len(c.Embedding) != 1 {
			t.Fatalf("missing embedding for chunk %s", c.ChunkID)
		}
		want := float32(len(c.ChunkText))
		if c.Embedding[0] != want {
			t.Fatalf("unexpected embedding value for chunk %s: got %f want %f", c.ChunkID, c.Embedding[0], want)
		}
	}

	expectedHash := diff.HashNormalizedText(diff.NormalizeText(stripHTML(p.Body.Storage.Value)))
	if state.PageID != p.ID {
		t.Fatalf("state page id mismatch: got %q want %q", state.PageID, p.ID)
	}
	if state.ParentPageID != "parent-1" {
		t.Fatalf("state parent mismatch: got %q want %q", state.ParentPageID, "parent-1")
	}
	if state.Version != p.Version.Number {
		t.Fatalf("state version mismatch: got %d want %d", state.Version, p.Version.Number)
	}
	if state.BodyNormHash != expectedHash {
		t.Fatalf("state hash mismatch: got %q want %q", state.BodyNormHash, expectedHash)
	}
	if !state.Exists {
		t.Fatalf("expected state.Exists to be true")
	}
}
