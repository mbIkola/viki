# 0004: confluence change intelligence skill

## Что добавлено

- Repo-local skill: `skills/confluence-change-intelligence/SKILL.md`
- Install script: `scripts/install-confluence-change-skill.sh`

Установка:

```bash
./scripts/install-confluence-change-skill.sh
```

## Routing policy

- Primary: `confluence_replica` MCP
- Fallback: upstream `confluence` MCP только при недоступности replica

## Smoke / Regression Checklist

Автоматический smoke:

```bash
./scripts/skill-confluence-change-smoke.py
```

1. Trigger phrase (RU):
   - `что изменилось в конфлюенсе пока меня не было`
   - Expected: route to `confluence_replica.what_changed`
2. Trigger phrase (EN):
   - `what changed in confluence this week`
   - Expected: route to `confluence_replica.what_changed`
3. Non-change intent:
   - `find onboarding page in confluence`
   - Expected: `confluence_replica.search/ask` (not necessarily `what_changed`)
4. Replica unavailable simulation:
   - stop/disable `confluence_replica` MCP
   - query: `what changed in confluence`
   - Expected: fallback to `confluence` MCP with explicit source note

## Ограничение v1

- Диаграммы (`drawio`) детектятся как metadata change, но их семантика не распознается.
