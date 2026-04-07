# confluence-replica

Local memory for Confluence.  
Fast retrieval, deterministic citations, and offline survivability when the network behaves like a liar.

## High-level idea

Most knowledge systems fail in the same way:

- they depend on a live upstream API
- they get slow under pressure
- they pretend confidence when context is thin

`confluence-replica` does the opposite. It pulls Confluence content into a local Postgres + pgvector index, then serves agents from local truth first.

Design intent:

- keep the ingestion/sync runtime internal
- expose a small, stable MCP facade for agents
- make retrieval fast, inspectable, and debuggable
- stay useful offline as long as local index exists

You do not need to mirror every internal endpoint into MCP.  
Agents need focused retrieval primitives, not another sprawling API to hallucinate over.

## Architecture

- Internal system:
  - Go services for ingest, diff, digest, search, and RAG orchestration
  - HTTP API (`cmd/api`) for internal service integration and ops workflows
- Agent facade:
  - MCP server (`cmd/mcp`) over stdio
  - retrieval-only surface for indexed knowledge access

Non-goal: expose `bootstrap/sync/digest jobs` through MCP.

## MCP v1 contract

### Binary

- `go run ./cmd/mcp --config config/config.yaml`
- MCP server implementation uses official SDK: `github.com/modelcontextprotocol/go-sdk`

### Resources

- `confluence://page/{page_id}`
- `confluence://chunk/{chunk_id}`
- `confluence://digest/{yyyy-mm-dd}`

### Tools

- `search(query, limit=10, include_snippets=true)`
  - returns ranked hits with `page_id`, `version`, `title`, `snippet`, `score`, and resource URIs
- `ask(query, top_k=8)`
  - deterministic synthesis over local retrieved snippets only
  - returns explicit citations and `refused=true` if context is weak
- `get_tree(root_page_id, depth=2, limit=200)`
  - returns local page subtree for context exploration
- `what_changed(date?, run_id?, parent_id?, limit=50, include_excerpts=true)`
  - returns structured per-page diffs captured during sync
  - includes change reasons and diagram metadata flags

### Prompts

- `daily_brief(date)`
- `investigate_page(page_id, question)`
- `compare_versions(page_id, from_version, to_version)`

### Why this shape

HTTP API is internal truth for services and operations.  
MCP is a narrow, stable entrance for agents.

## Add this MCP to Codex and Cline

### 1) Build the MCP binary

From `confluence-replica` directory:

- `make build-mcp`

This creates:

- `./bin/mcp`

### 2) Codex desktop config

Edit `~/.codex/config.toml` and add:

```toml
[mcp_servers.confluence_replica]
command = "/Users/${USER}/dev/codex/confluence-replica/bin/mcp"
args = ["--config", "/Users/${USER}/dev/codex/confluence-replica/config/config.yaml"]
```

Then restart Codex (or reload MCP servers in app settings).

Alternative without binary build:

```toml
[mcp_servers.confluence_replica]
command = "go"
args = ["run", "./cmd/mcp", "--config", "config/config.yaml"]
cwd = "/Users/${USER}/dev/codex/confluence-replica"
```

### 3) Cline config

Cline uses `cline_mcp_settings.json`.
Typical locations:

- Cline CLI: `~/.cline/data/settings/cline_mcp_settings.json`
- Cline VS Code extension (macOS):
  `/Users/<you>/Library/Application Support/Code/User/globalStorage/saoudrizwan.claude-dev/settings/cline_mcp_settings.json`

Add server entry:

```json
{
  "mcpServers": {
    "confluence-replica": {
      "command": "/Users/${USER}/dev/codex/confluence-replica/bin/mcp",
      "args": [
        "--config",
        "/Users/${USER}/dev/codex/confluence-replica/config/config.yaml"
      ],
      "disabled": false
    }
  }
}
```

Or via CLI:

- `cline mcp add confluence-replica -- /Users/${USER}/dev/codex/confluence-replica/bin/mcp --config /Users/${USER}/dev/codex/confluence-replica/config/config.yaml`

### 4) Smoke check

After connecting, verify in the client that MCP exposes:

- tools: `search`, `ask`, `get_tree`, `what_changed`
- resource templates: `confluence://page/{id}`, `confluence://chunk/{id}`, `confluence://digest/{date}`
- prompts: `daily_brief`, `investigate_page`, `compare_versions`

Local smoke without any AI client:

- `make mcp-smoke`

Or directly:

- `./scripts/mcp-smoke.py --config config/config.yaml`
- `./scripts/mcp-smoke.py --go-run --config config/config.yaml`

## Operations

### Runtime: Postgres + pgvector

1. Prepare compose env:
   - `cp .env.example .env`
2. Create host data dir:
   - `mkdir -p "${HOME}/.local/viki/confluence/postgres-data"`
3. Start database:
   - `docker compose up -d postgres`
4. Check health and logs:
   - `docker compose ps`
   - `docker compose logs postgres`
5. Apply migrations:
   - `make db-migrate`
6. Verify pgvector extension:
   - `docker compose exec -T postgres psql -U postgres -d confluence_replica -c "SELECT extname FROM pg_extension WHERE extname='vector';"`
7. Optional one-shot launcher:
   - `make runtime-up`

Local DSN:

- `postgres://postgres:postgres@localhost:5432/confluence_replica?sslmode=disable`

### Ollama embeddings (host-native)

- Endpoint: `http://127.0.0.1:11434`
- Model example: `nomic-embed-text`
- Config file: `config/config.yaml` (`embeddings.*`)
- Environment overrides: `OLLAMA_BASE_URL`, `OLLAMA_EMBED_MODEL`
- Pull model once: `ollama pull nomic-embed-text`

### Confluence PAT from Keychain

`config/config.yaml` supports secret refs in `confluence.token`:

- `keychain://codex_confluence_pat`
- `keychain://codex_confluence_pat?account=oracle-user`

Example to save PAT:

- `security add-generic-password -U -s codex_confluence_pat -a oracle-user -w "<your_pat_here>"`

### Makefile quickstart

- `make help`
- `make runtime-up`
- `make db-migrate`
- `make db-vector-check`
- `make api`
- `make bootstrap PARENT_ID=<page_id>`
- `make sync PARENT_ID=<page_id>`
- `make digest DATE=2026-04-07`

### Commands

- `go run ./cmd/replica bootstrap --config config/config.yaml --parent-id <page_id>`
- `go run ./cmd/replica sync --config config/config.yaml --parent-id <page_id>`
- `go run ./cmd/replica digest --config config/config.yaml --date 2026-04-07`
- `go run ./cmd/api --config config/config.yaml`
- `go run ./cmd/mcp --config config/config.yaml`
- `make mcp-smoke`

### Internal API v1

- `POST /search`
- `POST /chat`
- `GET /digest/{date}`
- `POST /jobs/bootstrap`
- `POST /jobs/sync`
- `POST /jobs/digest`

### Scheduler

Use external cron or CronJob:

- `sync` every N minutes
- `digest` every morning in local runtime timezone
