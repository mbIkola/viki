# confluence-replica

Локальная память для Confluence, чтобы агент отвечал по данным, а не по настроению.

Если кратко: это SQLite-реплика + MCP-сервер. Поднимаешь один бинарь, и агент получает нормальные `search`, `ask`, `what_changed` без ритуалов с `curl` по живым страницам.

## Почему это вообще существует

Потому что реальность скучно предсказуема.

> "Just ask your AI agent if she wants to use atlassian-mcp..."

Частый ответ: «да ну, давай я лучше `curl`-ом дерну нужные страницы».

Итог такого героизма: медленно, хрупко, теряется контекст, результат плавает между запусками. `confluence-replica` закрывает именно эту дыру.

## Что нужно для работы

Минимум внешних зависимостей:

- `Ollama`
- модель `nomic-embed-text`

Плюс локальные инструменты:

- `go` `1.25+`
- `gcc` (нужен из-за CGO для `sqlite`)

## Быстрый старт (для обычных смертных)

1. Подготовь конфиг:

```bash
cd confluence-replica
cp .env.example .env
cp config/config.example.yaml config/config.yaml
```

2. Положи Confluence PAT в macOS Keychain:

```bash
security add-generic-password -U -s codex_confluence_pat -a "$(whoami)" -w "<YOUR_PAT>"
```

3. Подними embeddings:

```bash
ollama pull nomic-embed-text
ollama serve
```

4. Собери MCP-бинарь и сделай первичную загрузку данных:

```bash
make build-mcp
make bootstrap
```

5. Запусти обновление индекса, когда нужно:

```bash
make sync
```

## Запуск MCP (что реально нужно агенту)

```bash
./bin/mcp --config config/config.yaml
```

Да, всё. Отдельный HTTP API-сервер обычному пользователю не нужен.

## Подключение в Codex

```toml
[mcp_servers.confluence_replica]
command = "/ABS/PATH/confluence-replica/bin/mcp"
args = ["--config", "/ABS/PATH/confluence-replica/config/config.yaml"]
```

## Что не нужно

- Docker в happy path
- Postgres / pgvector
- запускать `cmd/api`, если тебе нужен только MCP
- надежда, что агент сам выберет самый рациональный путь

## Технические детали (если очень хочется страдать)

Весь инженерный шум, контракты и архитектура вынесены в docs:

- `docs/technical-reference.md`
- `../docs/0003-confluence-replica-service-and-mcp.md`
