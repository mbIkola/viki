# Confluence MCP Write Tools Design (v1)

## Context

`confluence-replica` MCP currently provides retrieval-only tools (`search`, `ask`, `get_tree`, `what_changed`).
For page edits/creation, users must call upstream Confluence REST manually via `curl` + PAT, which is brittle and inconvenient.

This design adds minimal write capabilities to `confluence-replica` MCP while preserving the current local-replica-first architecture.

## Goals

1. Add MCP write functionality for:
   - update existing page (`title`/`body`, optional `version_message`)
   - create child page under a parent
2. Keep optimistic version behavior for update (MCP fetches current version; caller does not pass `expected_version`).
3. Immediately refresh local SQLite replica after successful remote write.
4. Keep scope intentionally narrow and safe:
   - body format in v1 is only Confluence storage XHTML
   - write tools are disabled by default via config flag

## Non-goals (v1)

1. Markdown input or markdown->XHTML conversion.
2. Move/archive/delete page operations.
3. Label mutation APIs.
4. Asynchronous write jobs.
5. General-purpose MCP mirror of upstream Confluence REST.

## Options Considered

### Option A (Chosen): MCP -> Confluence REST directly

- Add write tools in MCP server and route directly to extended `internal/confluence.Client`.
- After remote success, perform read-back and local upsert.

Pros:
- shortest path to value
- fewer moving parts and lower latency
- minimal infrastructure changes for MVP

Cons:
- write orchestration lives in MCP path, so boundaries must stay explicit

### Option B: MCP -> internal HTTP API -> Confluence REST

Pros:
- write logic centralized behind internal API

Cons:
- extra hop and complexity for no MVP benefit

### Option C: Async job model

Pros:
- retries/queue control for bulk operations

Cons:
- over-engineered for current use case

## High-level Architecture

Write flow remains inside the existing `cmd/mcp` runtime backend.

1. MCP tool validates input and checks `mcp.write_enabled`.
2. Backend calls `internal/confluence.Client` write method.
3. Backend reads back affected pages from Confluence.
4. Backend updates local SQLite via existing ingest-compatible mapping/chunking pipeline.
5. MCP returns structured result with new page version and local refresh status.

No new service/binary is introduced.

## Configuration Changes

Add a new config section:

```yaml
mcp:
  write_enabled: false
```

Behavior:

1. Default is `false`.
2. If `false`, write tools return `write disabled by config`.
3. `cmd/mcp` keeps local read compatibility (token may be absent when write is disabled), but startup validation must require a non-empty Confluence token when `mcp.write_enabled=true`.

## MCP Tool Contracts

## `update_page`

Input:

- `page_id` (required)
- `title` (optional)
- `body_storage` (optional, Confluence storage XHTML)
- `version_message` (optional)

Validation:

1. `page_id` required.
2. At least one of `title` or `body_storage` must be present.
3. Whitespace-only values are treated as missing.

Behavior:

1. Fetch current page from Confluence.
2. Compose update payload (missing fields inherit current values).
3. PUT with `version.number = current + 1`.
4. On version conflict, perform one automatic retry:
   - refetch current page
   - retry update with latest `current + 1`
5. Read-back updated page and upsert into local replica.

Output:

- `page_id`
- `title`
- `parent_page_id`
- `space_key`
- `new_version`
- `updated_at`
- `local_refreshed` (bool)

## `create_child_page`

Input:

- `parent_page_id` (required)
- `title` (required)
- `body_storage` (required, Confluence storage XHTML)
- `version_message` (optional)

Validation:

1. All required fields must be non-empty after trim.
2. `body_storage` accepts only storage XHTML in v1.

Behavior:

1. Fetch parent page (validate existence and resolve `space_key`).
2. POST create page with ancestor = parent.
3. Read-back created page.
4. Read-back parent page.
5. Upsert both created page and parent into local replica.

Output:

- `page_id` (new page)
- `title`
- `parent_page_id`
- `space_key`
- `new_version`
- `updated_at`
- `local_refreshed` (bool)

## Local Refresh Strategy

Local refresh uses immediate read-back + upsert, not delayed scheduler sync.

To avoid duplicated SQL paths, write backend should reuse ingest-compatible transformation logic:

1. page metadata mapping (`space`, `title`, `parent`, `status`, timestamps, labels)
2. body normalization and hashing
3. chunking (same chunk size/version)
4. embeddings for chunks
5. `UpsertPageWithVersion` in store

This keeps search/RAG index consistency with existing ingest behavior.

## Error Taxonomy

Tool-level errors should map to stable categories:

1. `validation_error`
2. `write_disabled`
3. `auth_error`
4. `upstream_error`
5. `version_conflict`
6. `local_refresh_failed`

Partial success rule:

- If remote write succeeds but local refresh fails, return error indicating partial success:
  - `remote_applied=true`
  - `local_refreshed=false`
  - explicit recommendation to run `sync`

This avoids false rollback semantics and preserves operator clarity.

## Components to Change

1. `internal/app/runtime.go`
   - extend config with `mcp.write_enabled`
   - add validation rule: `mcp.write_enabled=true` requires resolved non-empty Confluence token
2. `internal/confluence/client.go`
   - add POST/PUT helpers
   - add `UpdatePage` with one conflict retry
   - add `CreateChildPage`
3. `internal/mcp/server.go`
   - extend backend interface for write operations
   - add tools `update_page` and `create_child_page`
   - add input/output schemas and validation
4. `cmd/mcp/main.go`
   - implement runtime backend write methods
   - enforce `write_enabled` guard
   - orchestrate read-back + local upsert
5. `internal/ingest` (or adjacent reusable helper)
   - extract/share single-page upsert mapping logic to avoid divergence
6. Tests
   - MCP server tests for tool exposure/validation/errors
   - Confluence client tests for happy path + conflict retry + error parsing
   - Runtime backend tests for guard/refresh/partial-success behavior
   - integration smoke expected tool list update

## Testing Strategy

Unit tests:

1. `internal/mcp/server_test.go`
   - new tools listed
   - invalid payload rejection
   - disabled-write behavior
2. `internal/confluence/client` tests
   - update/create success
   - update conflict then successful retry
   - update conflict twice -> `version_conflict`
3. backend write tests
   - successful remote + successful local refresh
   - remote success + local refresh failure -> partial-success classification

Integration tests:

1. Extend MCP smoke test tool inventory to include write tools.
2. Keep existing read-only contract checks intact.

## Rollout Notes

1. Ship disabled by default (`mcp.write_enabled=false`).
2. Enable per environment only where PAT and operational ownership are clear.
3. Observe error rates for `version_conflict` and `local_refresh_failed` before expanding scope.

## Future Extensions (Post-v1)

1. Add dedicated markdown->storage conversion tool.
2. Add optional explicit concurrency contract (`expected_version`) for strict clients.
3. Add write audit trail in local DB for MCP-originated mutations.
4. Add move/archive/delete operations only after write-path reliability data is collected.
