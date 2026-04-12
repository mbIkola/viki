package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewSQLiteStoreBootstrapsSchemaAndPersistsProfile(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "replica.db")
	profile := testProfile()

	st, err := NewSQLiteStore(ctx, path, profile)
	if err != nil {
		t.Fatalf("unexpected error creating sqlite store: %v", err)
	}
	defer st.Close()

	got, err := st.IndexProfile(ctx)
	if err != nil {
		t.Fatalf("unexpected error reading profile: %v", err)
	}
	if got != profile {
		t.Fatalf("unexpected profile: %#v", got)
	}

	st.Close()

	reopened, err := NewSQLiteStore(ctx, path, profile)
	if err != nil {
		t.Fatalf("unexpected reopen error: %v", err)
	}
	defer reopened.Close()
}

func TestNewSQLiteStoreFailsOnProfileMismatch(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "replica.db")

	st, err := NewSQLiteStore(ctx, path, testProfile())
	if err != nil {
		t.Fatalf("unexpected create error: %v", err)
	}
	st.Close()

	_, err = NewSQLiteStore(ctx, path, IndexProfile{
		SchemaVersion:          testProfile().SchemaVersion,
		EmbeddingProvider:      "ollama",
		EmbeddingModel:         "different-model",
		EmbeddingDimension:     2,
		ChunkingVersion:        "runes900-v1",
		EmbeddingNormalization: "none",
	})
	if err == nil || !strings.Contains(err.Error(), "reindex required") {
		t.Fatalf("expected reindex required error, got %v", err)
	}
}

func TestSQLiteStoreUpsertAndSearch(t *testing.T) {
	ctx := context.Background()
	st := newTestSQLiteStore(t)
	defer st.Close()

	rootPage := Page{
		PageID:       "page-1",
		SpaceKey:     "OPS",
		Title:        "Runbook",
		ParentPageID: "",
		CurrentVer:   1,
		UpdatedAt:    time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
		CreatedAt:    time.Date(2026, 4, 11, 10, 0, 0, 0, time.UTC),
		PathHash:     "root",
		Tags:         []string{"incident"},
		Status:       "current",
	}
	version := PageVersion{
		PageID:     "page-1",
		Version:    1,
		AuthorID:   "user-1",
		BodyRaw:    "<p>alpha beta</p>",
		BodyNorm:   "alpha beta",
		BodyHash:   "body-hash-1",
		Title:      "Runbook",
		ParentPage: "",
		FetchedAt:  time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
	}
	chunks := []Chunk{
		{
			PageID:     "page-1",
			Version:    1,
			ChunkID:    "page-1:1:0",
			ChunkText:  "alpha beta",
			ChunkHash:  "chunk-hash-1",
			TokenCount: 2,
			Embedding:  []float32{1, 0},
		},
		{
			PageID:     "page-1",
			Version:    1,
			ChunkID:    "page-1:1:1",
			ChunkText:  "gamma delta",
			ChunkHash:  "chunk-hash-2",
			TokenCount: 2,
			Embedding:  []float32{0, 1},
		},
	}
	if err := st.UpsertPageWithVersion(ctx, rootPage, version, chunks); err != nil {
		t.Fatalf("unexpected upsert error: %v", err)
	}

	lexical, err := st.SearchLexical(ctx, "alpha", 10)
	if err != nil {
		t.Fatalf("unexpected lexical search error: %v", err)
	}
	if len(lexical) != 1 || lexical[0].ChunkID != "page-1:1:0" {
		t.Fatalf("unexpected lexical hits: %#v", lexical)
	}
	if lexical[0].Rank != 1 || lexical[0].RankValue == 0 {
		t.Fatalf("expected lexical rank metadata, got %#v", lexical[0])
	}
	if !strings.Contains(strings.ToLower(lexical[0].Snippet), "alpha") {
		t.Fatalf("expected lexical snippet to mention alpha, got %#v", lexical[0])
	}

	semantic, err := st.SearchSemantic(ctx, []float32{1, 0}, 10)
	if err != nil {
		t.Fatalf("unexpected semantic search error: %v", err)
	}
	if len(semantic) != 2 {
		t.Fatalf("expected two semantic hits, got %#v", semantic)
	}
	if semantic[0].ChunkID != "page-1:1:0" || semantic[0].Rank != 1 {
		t.Fatalf("expected closest chunk first, got %#v", semantic)
	}

	replacement := []Chunk{
		{
			PageID:     "page-1",
			Version:    1,
			ChunkID:    "page-1:1:0",
			ChunkText:  "zeta only",
			ChunkHash:  "chunk-hash-3",
			TokenCount: 2,
			Embedding:  []float32{0, 1},
		},
	}
	if err := st.UpsertPageWithVersion(ctx, rootPage, version, replacement); err != nil {
		t.Fatalf("unexpected replacement upsert error: %v", err)
	}

	lexical, err = st.SearchLexical(ctx, "alpha", 10)
	if err != nil {
		t.Fatalf("unexpected lexical search after replacement: %v", err)
	}
	if len(lexical) != 0 {
		t.Fatalf("expected replaced chunks to disappear from FTS, got %#v", lexical)
	}

	semantic, err = st.SearchSemantic(ctx, []float32{0, 1}, 10)
	if err != nil {
		t.Fatalf("unexpected semantic search after replacement: %v", err)
	}
	if len(semantic) != 1 || semantic[0].ChunkID != "page-1:1:0" {
		t.Fatalf("expected only replacement vector row to remain, got %#v", semantic)
	}
}

func TestSQLiteStoreReadAPIs(t *testing.T) {
	ctx := context.Background()
	st := newTestSQLiteStore(t)
	defer st.Close()

	page := Page{
		PageID:       "page-read",
		SpaceKey:     "OPS",
		Title:        "Readable",
		ParentPageID: "",
		CurrentVer:   2,
		UpdatedAt:    time.Date(2026, 4, 12, 11, 0, 0, 0, time.UTC),
		CreatedAt:    time.Date(2026, 4, 10, 11, 0, 0, 0, time.UTC),
		PathHash:     "readable-hash",
		Tags:         []string{"docs"},
		Status:       "current",
	}
	version := PageVersion{
		PageID:     "page-read",
		Version:    2,
		AuthorID:   "user-1",
		BodyRaw:    "<p>Readable body</p>",
		BodyNorm:   "Readable body",
		BodyHash:   "readable-body-hash",
		Title:      "Readable",
		ParentPage: "",
		FetchedAt:  time.Date(2026, 4, 12, 11, 0, 0, 0, time.UTC),
	}
	chunks := []Chunk{{
		PageID:     "page-read",
		Version:    2,
		ChunkID:    "page-read:2:0",
		ChunkText:  "Readable chunk",
		ChunkHash:  "readable-chunk-hash",
		TokenCount: 2,
	}}
	if err := st.UpsertPageWithVersion(ctx, page, version, chunks); err != nil {
		t.Fatalf("unexpected upsert error: %v", err)
	}

	doc, err := st.GetPageCurrent(ctx, "page-read")
	if err != nil {
		t.Fatalf("unexpected current page error: %v", err)
	}
	if doc.Version != 2 || doc.BodyHash != "readable-body-hash" {
		t.Fatalf("unexpected current page doc: %#v", doc)
	}
	if doc.Title != "Readable" || len(doc.Labels) != 1 || doc.Labels[0] != "docs" {
		t.Fatalf("unexpected current page metadata: %#v", doc)
	}

	chunk, err := st.GetChunk(ctx, "page-read:2:0")
	if err != nil {
		t.Fatalf("unexpected chunk error: %v", err)
	}
	if chunk.PageID != "page-read" || chunk.ChunkHash != "readable-chunk-hash" {
		t.Fatalf("unexpected chunk doc: %#v", chunk)
	}

	states, err := st.ListCurrentStates(ctx)
	if err != nil {
		t.Fatalf("unexpected current states error: %v", err)
	}
	if len(states) != 1 {
		t.Fatalf("expected one current state, got %#v", states)
	}
	if states[0].PageID != "page-read" || states[0].BodyNormHash != "readable-body-hash" {
		t.Fatalf("unexpected current state: %#v", states[0])
	}
}

func TestSQLiteStoreDigestTreeAndDiffQueries(t *testing.T) {
	ctx := context.Background()
	st := newTestSQLiteStore(t)
	defer st.Close()

	root := Page{
		PageID:       "root",
		SpaceKey:     "OPS",
		Title:        "Root",
		ParentPageID: "",
		CurrentVer:   3,
		UpdatedAt:    time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC),
		CreatedAt:    time.Date(2026, 4, 10, 9, 0, 0, 0, time.UTC),
		PathHash:     "root-hash",
		Tags:         []string{"ops"},
		Status:       "current",
	}
	child := Page{
		PageID:       "child",
		SpaceKey:     "OPS",
		Title:        "Child",
		ParentPageID: "root",
		CurrentVer:   1,
		UpdatedAt:    time.Date(2026, 4, 12, 9, 30, 0, 0, time.UTC),
		CreatedAt:    time.Date(2026, 4, 10, 9, 30, 0, 0, time.UTC),
		PathHash:     "child-hash",
		Tags:         []string{"ops"},
		Status:       "current",
	}
	rootVersion := PageVersion{
		PageID:     "root",
		Version:    3,
		AuthorID:   "user-1",
		BodyRaw:    "<p>root body</p>",
		BodyNorm:   "root body",
		BodyHash:   "root-body-hash",
		Title:      "Root",
		ParentPage: "",
		FetchedAt:  time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC),
	}
	childVersion := PageVersion{
		PageID:     "child",
		Version:    1,
		AuthorID:   "user-2",
		BodyRaw:    "<p>child body</p>",
		BodyNorm:   "child body",
		BodyHash:   "child-body-hash",
		Title:      "Child",
		ParentPage: "root",
		FetchedAt:  time.Date(2026, 4, 12, 9, 30, 0, 0, time.UTC),
	}
	if err := st.UpsertPageWithVersion(ctx, root, rootVersion, []Chunk{{PageID: "root", Version: 3, ChunkID: "root:3:0", ChunkText: "root body", ChunkHash: "root-chunk", TokenCount: 2}}); err != nil {
		t.Fatalf("unexpected root upsert error: %v", err)
	}
	if err := st.UpsertPageWithVersion(ctx, child, childVersion, []Chunk{{PageID: "child", Version: 1, ChunkID: "child:1:0", ChunkText: "child body", ChunkHash: "child-chunk", TokenCount: 2}}); err != nil {
		t.Fatalf("unexpected child upsert error: %v", err)
	}

	if err := st.SaveDigest(ctx, time.Date(2026, 4, 12, 0, 0, 0, 0, time.UTC), "# Digest", map[string]any{"total": 1}); err != nil {
		t.Fatalf("unexpected save digest error: %v", err)
	}
	digest, err := st.GetDigest(ctx, time.Date(2026, 4, 12, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("unexpected get digest error: %v", err)
	}
	if digest != "# Digest" {
		t.Fatalf("unexpected digest markdown: %q", digest)
	}

	runID, err := st.BeginSyncRun(ctx, "sync")
	if err != nil {
		t.Fatalf("unexpected begin sync error: %v", err)
	}
	if err := st.InsertChangeEvents(ctx, []ChangeEvent{{
		RunID:      runID,
		PageID:     "child",
		Type:       "updated",
		OldVersion: 0,
		NewVersion: 1,
		Summary:    "child updated",
	}}); err != nil {
		t.Fatalf("unexpected change event insert error: %v", err)
	}
	day := time.Date(2026, 4, 12, 0, 0, 0, 0, time.UTC)
	if err := st.InsertPageChangeDiffs(ctx, []PageChangeDiff{{
		RunID:           runID,
		PageID:          "child",
		OldVersion:      0,
		NewVersion:      1,
		ChangeKind:      "updated",
		BodyNormChanged: true,
		Summary:         "changed",
		Excerpts: ChangeExcerpts{
			Before: "before",
			After:  "after",
			Source: "body_norm",
		},
	}}); err != nil {
		t.Fatalf("unexpected page diff insert error: %v", err)
	}
	if err := st.FinishSyncRun(ctx, runID, "success", map[string]any{"pages": 2}); err != nil {
		t.Fatalf("unexpected finish sync error: %v", err)
	}

	events, err := st.ListChangeEventsForDate(ctx, day)
	if err != nil {
		t.Fatalf("unexpected list events error: %v", err)
	}
	if len(events) != 1 || events[0].PageID != "child" {
		t.Fatalf("unexpected events: %#v", events)
	}

	diffs, err := st.ListPageChangeDiffs(ctx, PageChangeDiffQuery{
		Date:         &day,
		RunID:        runID,
		ParentPageID: "root",
		Limit:        10,
	})
	if err != nil {
		t.Fatalf("unexpected list diffs error: %v", err)
	}
	if len(diffs) != 1 || diffs[0].PageID != "child" {
		t.Fatalf("unexpected diffs: %#v", diffs)
	}

	tree, err := st.GetTree(ctx, "root", 2, 10)
	if err != nil {
		t.Fatalf("unexpected tree error: %v", err)
	}
	if len(tree) != 2 {
		t.Fatalf("expected root and child nodes, got %#v", tree)
	}
	if tree[0].PageID != "root" {
		t.Fatalf("expected root first, got %#v", tree)
	}
	if tree[1].PageID != "child" || tree[1].Depth != 1 {
		t.Fatalf("expected child at depth 1, got %#v", tree[1])
	}
}

func newTestSQLiteStore(t *testing.T) *SQLiteStore {
	t.Helper()

	st, err := NewSQLiteStore(context.Background(), filepath.Join(t.TempDir(), "replica.db"), testProfile())
	if err != nil {
		t.Fatalf("create sqlite store: %v", err)
	}
	return st
}

func testProfile() IndexProfile {
	return IndexProfile{
		SchemaVersion:          1,
		EmbeddingProvider:      "ollama",
		EmbeddingModel:         "nomic-embed-text",
		EmbeddingDimension:     2,
		ChunkingVersion:        "runes900-v1",
		EmbeddingNormalization: "none",
	}
}
