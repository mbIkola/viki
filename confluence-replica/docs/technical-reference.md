# confluence-replica technical reference

This file keeps the detailed architecture/runtime contract that used to live in `README.md`.
If you just want to run MCP quickly, read `../README.md` instead.

Local memory for Confluence, backed by a single SQLite database file.

`confluence-replica` keeps ingest, diff, digest, search, and RAG on a local index first. The local runtime is intentionally simple:

- one SQLite file at `database.path`
- `FTS5` for lexical ranking and native `snippet()` output
- `sqlite-vec` `vec0` for semantic KNN search
- Reciprocal Rank Fusion in Go over top lexical and vector candidates

No Postgres. No pgvector. No Docker in the happy path.

## Architecture

- Internal runtime:
  - Go services for bootstrap, sync, diff, digest, search, and deterministic RAG
  - HTTP API in `cmd/api` for internal service and ops flows
- Agent facade:
  - MCP server in `cmd/mcp` over stdio
  - read-first surface for indexed knowledge, plus guarded Confluence write tools

Search behavior:

- lexical search stays inside SQLite with `MATCH`, `ORDER BY rank`, and `snippet()`
- semantic search stays inside SQLite `vec0` with cosine distance
- fusion is simple `1 / (60 + rank)` rank fusion in Go over top `50` lexical hits plus top `50` vector hits

## Local Runtime Contract

The SQLite runtime is the only supported local runtime.

- `database.path` replaces `database.dsn`
- the parent directory and database file are created automatically on first start
- schema bootstrap happens at startup from a single embedded schema
- incompatible index changes are handled by rebuild, not migrations

`replica_meta` stores the active index profile:

- `schema_version`
- `embedding_provider`
- `embedding_model`
- `embedding_dimension`
- `chunking_version`
- `embedding_normalization`

Startup validates this profile exactly. If it changes, startup fails fast with `reindex required`.

## Prerequisites

- Go `1.25+`
- `gcc` and `CGO_ENABLED=1`
- SQLite build tag `sqlite_fts5`
- optional Ollama if you want semantic embeddings locally

The project uses:

- `github.com/mattn/go-sqlite3`
- `github.com/asg017/sqlite-vec-go-bindings/cgo`

Treat that CGO toolchain as part of the product contract. Trading Docker drama for hidden compiler drama would be a rather silly bargain.

## Config

Start from the example:

```bash
cp config/config.example.yaml config/config.yaml
```

Key fields:

- `database.path`
  - leave empty to use the default: `${HOME}/.local/viki/confluence/replica.db`
  - or set an absolute SQLite path explicitly
- `confluence.parent_ids`
  - source of truth for full bootstrap and sync scope
- `embeddings.*`
  - configure Ollama or set `provider: "noop"` to disable semantic embeddings

Optional `.env` overrides can be copied from `.env.example`.

## Quickstart

```bash
cp .env.example .env
cp config/config.example.yaml config/config.yaml
make test
make build
make bootstrap
```

Useful commands:

- `make bootstrap`
- `make sync`
- `make rebuild`
- `make digest DATE=2026-04-07`
- `make api`
- `make mcp`
- `make mcp-integration`

All `make` targets already use `CGO_ENABLED=1` and `-tags "sqlite_fts5"`.

## Rebuild Workflow

`rebuild` is the canonical reset, reindex, and repair path.

```bash
make rebuild
make rebuild PARENT_ID=925576634
make rebuild PARENT_IDS=925576634,776923811
```

Under the hood, `replica rebuild`:

- deletes `database.path`
- deletes sibling `-wal` and `-shm` files if present
- bootstraps a fresh schema
- writes a fresh `replica_meta`
- repulls the configured Confluence roots

Cold rebuild from Confluence is the only supported migration and recovery story.

## Direct Commands

If you prefer raw Go commands:

```bash
CGO_ENABLED=1 go run -tags sqlite_fts5 ./cmd/replica bootstrap --config config/config.yaml
CGO_ENABLED=1 go run -tags sqlite_fts5 ./cmd/replica sync --config config/config.yaml
CGO_ENABLED=1 go run -tags sqlite_fts5 ./cmd/replica rebuild --config config/config.yaml
CGO_ENABLED=1 go run -tags sqlite_fts5 ./cmd/replica digest --config config/config.yaml --date 2026-04-07
CGO_ENABLED=1 go run -tags sqlite_fts5 ./cmd/api --config config/config.yaml
CGO_ENABLED=1 go run -tags sqlite_fts5 ./cmd/mcp --config config/config.yaml
```

Full verification:

```bash
CGO_ENABLED=1 go test -tags sqlite_fts5 ./...
CGO_ENABLED=1 go test -tags "integration sqlite_fts5" ./integration -run TestMCPBinarySmoke -count=1
CGO_ENABLED=1 go build -tags sqlite_fts5 ./cmd/api ./cmd/replica ./cmd/mcp
```

## Ollama Embeddings

- endpoint default: `http://127.0.0.1:11434`
- model example: `nomic-embed-text`
- pull once with `ollama pull nomic-embed-text`
- environment overrides: `OLLAMA_BASE_URL`, `OLLAMA_EMBED_MODEL`

If embeddings are enabled, startup probes the model once to determine the exact embedding dimension and stores it in `replica_meta`.

## Confluence PAT from Keychain

`config/config.yaml` supports secret refs in `confluence.token`:

- `keychain://codex_confluence_pat`
- `keychain://codex_confluence_pat?account=oracle-user`

Example:

```bash
security add-generic-password -U -s codex_confluence_pat -a oracle-user -w "<your_pat_here>"
```

## MCP v1 Contract

Binary:

- `./bin/mcp`
- or `CGO_ENABLED=1 go run -tags sqlite_fts5 ./cmd/mcp --config config/config.yaml`

Resources:

- `confluence://page/{page_id}`
- `confluence://chunk/{chunk_id}`
- `confluence://digest/{yyyy-mm-dd}`

Tools:

- `search(query, limit=10, include_snippets=true)`
- `ask(query, top_k=8)`
- `get_tree(root_page_id, depth=2, limit=200)`
- `what_changed(date?, run_id?, parent_id?, limit=50, include_excerpts=true)`
- `update_page(page_id, title?, body_storage?)` (requires `page_id` and at least one of `title`/`body_storage`; MCP does lightweight pre-validation and returns `validation_error` for obviously invalid inputs)
- `create_child_page(parent_page_id, title, body_storage)` (`body_storage` is required; MCP applies a lightweight storage-shape check before sending upstream)

Write safety:

- `mcp.write_enabled` defaults to `false`
- when write tools are disabled, MCP returns `write_disabled`
- when `mcp.write_enabled=true`, `confluence.base_url` and a Confluence token are required
- MCP storage checks are heuristic pre-validation only; Confluence API remains the source of truth for strict storage XHTML validation
- MCP write failures are returned as tool-call errors with stable message tokens: `write_disabled`, `local_refresh_failed`, `version_conflict`, `auth_error`, `upstream_error`

Prompts:

- `daily_brief(date)`
- `investigate_page(page_id, question)`
- `compare_versions(page_id, from_version, to_version)`

## Add This MCP to Codex or Cline

Build the binary first:

```bash
make build-mcp
```

Codex desktop example:

```toml
[mcp_servers.confluence_replica]
command = "/Users/${USER}/dev/codex/confluence-replica/bin/mcp"
args = ["--config", "/Users/${USER}/dev/codex/confluence-replica/config/config.yaml"]
```

Alternative without a prebuilt binary:

```toml
[mcp_servers.confluence_replica]
command = "go"
args = ["run", "-tags", "sqlite_fts5", "./cmd/mcp", "--config", "config/config.yaml"]
cwd = "/Users/${USER}/dev/codex/confluence-replica"
env = { CGO_ENABLED = "1" }
```

Smoke test locally:

```bash
make mcp-integration
```

## Internal API v1

- `POST /search`
- `POST /chat`
- `GET /digest/{date}`
- `POST /jobs/bootstrap`
- `POST /jobs/sync`
- `POST /jobs/digest`

## Scheduler

Use external cron or a job runner:

- `sync` every N minutes
- `digest` every morning in local runtime timezone
