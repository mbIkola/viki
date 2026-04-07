---
name: confluence-change-intelligence
description: Prefer confluence_replica MCP for Confluence updates/history questions, with strict fallback to upstream confluence MCP only when replica is unavailable.
---

# Confluence Change Intelligence

Use this skill when the user asks about recent changes, updates, or summaries from Confluence.

## Routing Policy (Strict)

1. Try `confluence_replica` MCP first.
2. Only fallback to upstream `confluence` MCP when replica is unavailable:
   - tool not found
   - MCP session/transport down
   - timeout / connection failure
3. Do not fallback just because answer quality seems weak.
4. In the final answer, explicitly state source:
   - `Source: confluence_replica`
   - or `Source: confluence (fallback; replica unavailable)`

## Trigger Detection (High Recall)

Treat request as Confluence-change intent when it contains any of:

- RU patterns:
  - `что изменилось` + (`confluence`|`конфлю`|`вики`)
  - `что нового` + (`confluence`|`конфлю`|`вики`)
  - `что писали` + (`в confluence`|`в конфлю`|`в вики`)
  - `пока меня не было` + (`confluence`|`конфлю`|`вики`)
- EN patterns:
  - `what changed` + (`confluence`|`wiki`)
  - `what's new` + (`confluence`|`wiki`)
  - `updates in` + (`confluence`|`wiki`)
  - `catch me up` + (`confluence`|`wiki`)

When in doubt, treat it as a trigger.

## Tool Selection

- For explicit "what changed / what is new":
  - Call `confluence_replica.what_changed`.
- For exploratory/context questions:
  - Use `confluence_replica.search`, `confluence_replica.ask`, `confluence_replica.get_tree`.

## Output Expectations

- Give concise summary first, then key bullets.
- Include page IDs/titles/versions when available.
- If diff includes diagram-only metadata changes, explicitly mark:
  - `diagram_change_detected=true`
  - `diagram_content_unparsed=true`

## Known Limitation (v1)

Diagram internals are not parsed semantically yet. The system can detect diagram metadata changes, but cannot fully explain diagram content changes.
