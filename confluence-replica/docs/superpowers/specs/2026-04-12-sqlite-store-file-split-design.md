# SQLite Store File Split Design

Date: 2026-04-12
Status: Proposed

## Goal

Split `internal/store/sqlite.go` into smaller files inside the same `store` package so the SQLite implementation is easier to read and maintain without changing behavior, package boundaries, or public interfaces.

## Non-Goals

- No new subpackages.
- No internal repository structs.
- No behavior changes.
- No SQL rewrites except tiny local cleanup required by move-only refactoring.
- No store interface changes.

## Constraints

- Keep `package store` everywhere.
- Keep `SQLiteStore` as the concrete implementation type.
- Keep all existing tests passing.
- Prefer move-only edits over logical rewrites.

## Problem

`internal/store/sqlite.go` currently mixes:

- SQLite bootstrap and profile validation
- sync-run lifecycle
- page/version/chunk upsert logic
- change and digest persistence
- lexical and semantic search
- page, chunk, and tree read APIs
- generic SQLite/time helpers

That makes the file large, linear, and harder to hold in working memory.

## Design

Use file-level separation by data domain, keeping all code in one package.

### Target Layout

- `internal/store/store.go`
  - store contract types
  - `Store` interface
  - JSON/null helper functions already shared across implementations
  - `IndexProfile`

- `internal/store/sqlite_store.go`
  - `SQLiteStore`
  - `NewSQLiteStore`
  - `Close`
  - `Path`
  - `IndexProfile`

- `internal/store/sqlite_bootstrap.go`
  - `sqliteDSN`
  - `configureSQLite`
  - `bootstrapSchema`
  - `ensureVectorTable`
  - `ensureIndexProfile`
  - `readIndexProfile`
  - embedded schema declarations

- `internal/store/sqlite_sync.go`
  - `BeginSyncRun`
  - `FinishSyncRun`
  - `UpsertPageWithVersion`
  - `ListCurrentStates`
  - `chunkRowIDsByVersion`

- `internal/store/sqlite_changes.go`
  - `InsertChangeEvents`
  - `InsertPageChangeDiffs`
  - `SaveDigest`
  - `GetDigest`
  - `ListChangeEventsForDate`
  - `ListPageChangeDiffs`

- `internal/store/sqlite_search.go`
  - `SearchLexical`
  - `SearchSemantic`

- `internal/store/sqlite_docs.go`
  - `GetPageCurrent`
  - `GetPageVersion`
  - `GetChunk`
  - `GetTree`

- `internal/store/sqlite_helpers.go`
  - `currentTimestamp`
  - `storeTime`
  - `mustParseTime`

## Why This Shape

This keeps the package flat and boring, which is precisely the virtue here.

- Readers can open only the domain they care about.
- Shared types stay local and cheap to access.
- No import churn or package-boundary tax.
- The refactor remains mostly mechanical, which lowers regression risk.

## Implementation Plan

1. Move `IndexProfile` from `sqlite.go` into `store.go`.
2. Create the new SQLite files listed above.
3. Move methods and helpers into their target files without changing signatures.
4. Delete the old monolithic `sqlite.go`.
5. Run package tests, full test suite, and build verification.

## Verification

- `go test -tags sqlite_fts5 ./internal/store`
- `go test -tags sqlite_fts5 ./...`
- `go build -tags sqlite_fts5 ./cmd/api ./cmd/replica ./cmd/mcp`

## Risks

- Accidentally duplicating helper functions while moving code.
- Breaking embedded schema wiring if `go:embed` moves carelessly.
- Creating artificial cross-file dependencies that are worse than the original file.

## Mitigations

- Keep `go:embed schema.sql` in the bootstrap file only.
- Move code in small groups with no signature changes.
- Run verification after the split, not merely package compilation.
