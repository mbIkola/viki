# SQLite Store File Split Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Split `internal/store/sqlite.go` into focused files inside `package store` without changing behavior, public interfaces, or SQLite schema wiring.

**Architecture:** Keep a flat `store` package and move code by responsibility only: store construction, bootstrap/schema, sync writes, change/digest queries, search, document reads, and generic helpers. This remains a move-only refactor, with shared helpers staying package-local so call sites and imports stay stable.

**Tech Stack:** Go, `database/sql`, `github.com/mattn/go-sqlite3`, `github.com/asg017/sqlite-vec-go-bindings/cgo`, `go test`, `gofmt`

---

### Task 1: Characterization Coverage For SQLite Store APIs

**Files:**
- Modify: `internal/store/store_test.go`
- Test: `internal/store/store_test.go`

- [ ] **Step 1: Add a focused test for read APIs not explicitly covered yet**

```go
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

	if err := st.UpsertPageWithVersion(ctx, page, version, []Chunk{{
		PageID:     "page-read",
		Version:    2,
		ChunkID:    "page-read:2:0",
		ChunkText:  "Readable chunk",
		ChunkHash:  "readable-chunk-hash",
		TokenCount: 2,
	}}); err != nil {
		t.Fatalf("unexpected upsert error: %v", err)
	}

	doc, err := st.GetPageCurrent(ctx, "page-read")
	if err != nil {
		t.Fatalf("unexpected current page error: %v", err)
	}
	if doc.Version != 2 || doc.BodyHash != "readable-body-hash" {
		t.Fatalf("unexpected current page doc: %#v", doc)
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
	if len(states) != 1 || states[0].BodyNormHash != "readable-body-hash" {
		t.Fatalf("unexpected current states: %#v", states)
	}
}
```

- [ ] **Step 2: Run the focused store test before refactoring**

Run: `go test -tags sqlite_fts5 ./internal/store -run 'TestSQLiteStore(ReadAPIs|UpsertAndSearch|DigestTreeAndDiffQueries)'`
Expected: PASS, proving the current store behavior is characterized before the move-only refactor.

- [ ] **Step 3: Keep the new test as regression coverage during the split**

```go
// No production changes in this task. The test stays in place while files move.
```

### Task 2: Split SQLite Store Code By Responsibility

**Files:**
- Modify: `internal/store/store.go`
- Create: `internal/store/sqlite_store.go`
- Create: `internal/store/sqlite_bootstrap.go`
- Create: `internal/store/sqlite_sync.go`
- Create: `internal/store/sqlite_changes.go`
- Create: `internal/store/sqlite_search.go`
- Create: `internal/store/sqlite_docs.go`
- Create: `internal/store/sqlite_helpers.go`
- Delete: `internal/store/sqlite.go`

- [ ] **Step 1: Move shared store contract data into `store.go`**

```go
type IndexProfile struct {
	SchemaVersion          int
	EmbeddingProvider      string
	EmbeddingModel         string
	EmbeddingDimension     int
	ChunkingVersion        string
	EmbeddingNormalization string
}
```

- [ ] **Step 2: Create `sqlite_store.go` for construction and identity methods**

```go
type SQLiteStore struct {
	db      *sql.DB
	path    string
	profile IndexProfile
}

func NewSQLiteStore(ctx context.Context, path string, profile IndexProfile) (*SQLiteStore, error) { /* moved as-is */ }
func (s *SQLiteStore) Close()                                                { /* moved as-is */ }
func (s *SQLiteStore) Path() string                                          { /* moved as-is */ }
func (s *SQLiteStore) IndexProfile(ctx context.Context) (IndexProfile, error) { /* moved as-is */ }
```

- [ ] **Step 3: Move bootstrap/schema code into `sqlite_bootstrap.go`**

```go
//go:embed schema.sql
var schemaFS embed.FS

var sqliteAutoOnce sync.Once

func sqliteDSN(path string) string                               { /* moved as-is */ }
func configureSQLite(ctx context.Context, db *sql.DB) error      { /* moved as-is */ }
func bootstrapSchema(ctx context.Context, db *sql.DB) error      { /* moved as-is */ }
func ensureVectorTable(ctx context.Context, db *sql.DB, profile IndexProfile) error {
	/* moved as-is */
}
func ensureIndexProfile(ctx context.Context, db *sql.DB, profile IndexProfile) error {
	/* moved as-is */
}
func readIndexProfile(ctx context.Context, db *sql.DB) (IndexProfile, error) { /* moved as-is */ }
```

- [ ] **Step 4: Move sync, change, search, document, and helper functions into domain files**

```go
func (s *SQLiteStore) BeginSyncRun(ctx context.Context, mode string) (int64, error) { /* moved as-is */ }
func (s *SQLiteStore) FinishSyncRun(ctx context.Context, runID int64, status string, stats map[string]any) error {
	/* moved as-is */
}
func (s *SQLiteStore) UpsertPageWithVersion(ctx context.Context, p Page, v PageVersion, chunks []Chunk) error {
	/* moved as-is */
}
func chunkRowIDsByVersion(ctx context.Context, tx *sql.Tx, pageID string, version int) ([]int64, error) {
	/* moved as-is */
}
func currentTimestamp() string      { /* moved as-is */ }
func storeTime(t time.Time) string  { /* moved as-is */ }
func mustParseTime(raw string) time.Time { /* moved as-is */ }
```

- [ ] **Step 5: Remove the original monolith once all moved code compiles from the new files**

```go
// Delete internal/store/sqlite.go after every function and import has a new home.
```

### Task 3: Format And Verify The Refactor

**Files:**
- Modify: `internal/store/store.go`
- Modify: `internal/store/store_test.go`
- Verify: `internal/store/*.go`
- Verify: `cmd/api`, `cmd/replica`, `cmd/mcp`

- [ ] **Step 1: Format the touched Go files**

Run: `gofmt -w internal/store/store.go internal/store/store_test.go internal/store/sqlite_store.go internal/store/sqlite_bootstrap.go internal/store/sqlite_sync.go internal/store/sqlite_changes.go internal/store/sqlite_search.go internal/store/sqlite_docs.go internal/store/sqlite_helpers.go`
Expected: no output

- [ ] **Step 2: Run focused store verification**

Run: `go test -tags sqlite_fts5 ./internal/store`
Expected: PASS

- [ ] **Step 3: Run repository-wide verification from the spec**

Run: `go test -tags sqlite_fts5 ./...`
Expected: PASS

- [ ] **Step 4: Run build verification for shipped binaries**

Run: `go build -tags sqlite_fts5 ./cmd/api ./cmd/replica ./cmd/mcp`
Expected: exit code 0
