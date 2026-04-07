# 0002: confluence mcp smoketest

## Контекст

- Confluence type: Data Center / Server
- URL: `https://gbuconfluence.oraclecorp.com/`
- Target space: `UGBUPD`
- Auth: Personal Access Token (TTL 5 days)
- Access: only via VPN/proxy

## Что проверили (2026-04-07)

1. Базовый API reachable через корпоративный proxy.
2. Read API с PAT работает:
   - `GET /rest/api/space?limit=5` -> `200`
   - `GET /rest/api/space?spaceKey=UGBUPD&limit=25` -> `200`, space найден.
3. MCP client package поднимается локально:
   - `uvx mcp-atlassian --help` -> успешно, поддерживает `--confluence-personal-token`.

## Минимальный безопасный шаблон MCP (read-only)

```toml
[mcp_servers.confluence]
command = "uvx"
args = [
  "mcp-atlassian",
  "--transport", "stdio",
  "--read-only",
  "--confluence-url", "https://gbuconfluence.oraclecorp.com/",
  "--confluence-personal-token", "${CONFLUENCE_PAT}",
  "--confluence-spaces-filter", "UGBUPD"
]
```

Примечание: токен не хранить в `git`. Перед запуском экспортировать в окружение:

```bash
export CONFLUENCE_PAT="<new_pat>"
```

## Текущая реализация (2026-04-07)

- Добавлен запускатор: [`scripts/start-confluence-mcp.sh`](./dev/codex/scripts/start-confluence-mcp.sh)
  - Берет PAT из Keychain service `codex_confluence_pat` (или из `CONFLUENCE_PAT`).
  - Стартует `mcp-atlassian` в `read-only` режиме для space `UGBUPD`.
  - Приоритет запуска: системный `mcp-atlassian` (например, установленный через `brew`), затем `uvx` как fallback.
- В `~/.codex/config.toml` добавлен блок:

```toml
[mcp_servers.confluence]
command = "/Users/${USER}/dev/codex/scripts/start-confluence-mcp.sh"
args = []
```

### Установка MCP-бинарника (предпочтительно через Homebrew)

```bash
brew install mcp-atlassian
```

## Наблюдения

- Без PAT на конкретный `space` возможен `404` вместо явного `401`.
- На этой инсталляции скорость и доступ зависят от VPN/proxy.
- Практичнее держать сначала read-only MCP, write включать позже точечно.
