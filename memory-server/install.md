# Install memory-server

Upstream repo:

https://github.com/doobidoo/mcp-memory-service

## 1. Установить сервис

Самый простой вариант из upstream README:

```bash
pip install mcp-memory-service
```

Если нужен локальный checkout:

```bash
git clone https://github.com/doobidoo/mcp-memory-service.git
cd mcp-memory-service
python scripts/installation/install.py
```

## 2. Поднять HTTP service на нашем порту

У нас дефолтный порт для memory-server: `28000`.

```bash
MCP_MEMORY_STORAGE_BACKEND=sqlite_vec \
MCP_HTTP_PORT=28000 \
MCP_ALLOW_ANONYMOUS_ACCESS=true \
MCP_API_KEY="<local-dev-api-key>" \
memory server --http
```

Важно: `MCP_ALLOW_ANONYMOUS_ACCESS=true` дает anonymous-доступ только на чтение. Чтобы создавать memories из dashboard или через `POST /api/memories`, нужен API key.

Проверка:

```bash
curl http://127.0.0.1:28000/api/health
```

Ожидаемый ответ:

```json
{"status":"healthy"}
```

## 3. Добавить MCP в Codex

Открыть:

```bash
~/.codex/config.toml
```

Добавить:

```toml
[mcp_servers.memory]
command = "memory"
args = ["server"]

[mcp_servers.memory.env]
MCP_MEMORY_STORAGE_BACKEND = "sqlite_vec"
MCP_HTTP_PORT = "28000"
MCP_API_KEY = "<local-dev-api-key>"
MCP_MEMORY_BASE_DIR = "/Users/nkharche/Library/Application Support/mcp-memory"
MCP_MEMORY_SQLITE_PATH = "/Users/nkharche/Library/Application Support/mcp-memory/sqlite_vec.db"
MCP_MEMORY_BACKUPS_PATH = "/Users/nkharche/Library/Application Support/mcp-memory/backups"
```

Если `memory` не находится в PATH, используй абсолютный путь к Python из venv:

```toml
[mcp_servers.memory]
command = "/path/to/mcp-memory-service/.venv/bin/python"
args = ["-m", "mcp_memory_service.server"]
```

После этого перезапустить Codex, чтобы он перечитал MCP config. Как обычно, без перезапуска новые инструменты могут существовать лишь в воображении.

Для dashboard открыть `http://127.0.0.1:28000`, ввести тот же API key в auth modal и нажать Authenticate.
