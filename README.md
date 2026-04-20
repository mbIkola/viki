# codex bootstrap notes

Локальная база знаний по настройке рабочего окружения `codex`.

## Scope (now)

- Только документация.
- Без скриптов, без автоматизации, без привязки к конкретному репозиторию.

## Notes

- Skills: [`docs/skills/0001-superpowers.md`](./docs/skills/0001-superpowers.md)
- Confluence: [`docs/skills/0002-confluence-mcp-smoketest.md`](./docs/skills/0002-confluence-mcp-smoketest.md)

## `confluence_replica` in one paragraph

`confluence_replica` is a local SQLite mirror + MCP facade for selected Confluence roots.
It gives the agent fast, deterministic `search` / `ask` / `what_changed` over local data instead of improvising against live pages every single time.

### Why this exists

Because reality is predictable:

> "Just ask your AI agent if she wants to use atlassian-mcp..."

In practice, many agents run this loop:

- try `atlassian-mcp`
- fail
- clone/fix `atlassian-mcp` in `/tmp`
- run patched variant
- fail again on the next request
- declare MCP cursed and switch to `curl`

`confluence_replica` exists to eliminate that recurring hero story: no live MCP firefighting, no ad-hoc patching, no ritual fallback to `curl`.
