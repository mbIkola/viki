# confluence-replica

Local Confluence replica with hybrid search, daily diff digest, and RAG chat over local index.

## Runtime: Postgres + pgvector

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
   - `cat migrations/001_init.sql | docker compose exec -T postgres psql -U postgres -d confluence_replica`
6. Verify pgvector extension:
   - `docker compose exec -T postgres psql -U postgres -d confluence_replica -c "SELECT extname FROM pg_extension WHERE extname='vector';"`
7. Optional one-shot launcher:
   - `make runtime-up` (starts postgres, starts Ollama if needed, pulls embedding model if missing)

Local DSN:

- `postgres://postgres:postgres@localhost:5432/confluence_replica?sslmode=disable`

## Ollama embeddings (host-native)

Use Ollama outside Docker for best performance on macOS ARM:

- Endpoint: `http://127.0.0.1:11434`
- Model example: `nomic-embed-text`
- Config file: `config/config.yaml` (`embeddings.*` block)
- Environment overrides: `OLLAMA_BASE_URL`, `OLLAMA_EMBED_MODEL`
- Pull model once (if missing): `ollama pull nomic-embed-text`

## Confluence PAT from Keychain

`config/config.yaml` supports secret refs in `confluence.token`:

- `keychain://codex_confluence_pat`
- `keychain://codex_confluence_pat?account=oracle-user`

Runtime resolves these via macOS `security find-generic-password`.

Example to save PAT:

- `security add-generic-password -U -s codex_confluence_pat -a oracle-user -w "<your_pat_here>"`

Plain token strings are still supported if you do not use keychain refs.

## Makefile quickstart

- `make help`
- `make runtime-up`
- `make db-migrate`
- `make db-vector-check`
- `make api`
- `make bootstrap PARENT_ID=<page_id>`
- `make sync PARENT_ID=<page_id>`
- `make digest DATE=2026-04-07`

## Commands

- `go run ./cmd/replica bootstrap --config config/config.yaml --parent-id <page_id>`
- `go run ./cmd/replica sync --config config/config.yaml --parent-id <page_id>`
- `go run ./cmd/replica digest --config config/config.yaml --date 2026-04-07`
- `go run ./cmd/api`

## API v1

- `POST /search`
- `POST /chat`
- `GET /digest/{date}`
- `POST /jobs/bootstrap`
- `POST /jobs/sync`
- `POST /jobs/digest`

## Startup order

- Start `postgres` (`docker compose up -d postgres`)
- Apply SQL migration(s)
- Start `replica` / `api`

## Ops

Backup:

- `docker compose exec -T postgres pg_dump -U postgres -d confluence_replica > backup.sql`

Restore:

- `cat backup.sql | docker compose exec -T postgres psql -U postgres -d confluence_replica`

## Scheduler

Use external cron or CronJob:

- `sync` every N minutes
- `digest` every morning in local runtime timezone
