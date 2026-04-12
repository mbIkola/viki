# Changelog

All notable changes to `confluence-replica` are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html)
using namespaced monorepo tags: `confluence-replica/vX.Y.Z`.

## [Unreleased]

### Added

- _No notable changes yet._

### Changed

- _No notable changes yet._

### Fixed

- _No notable changes yet._

### Removed

- _No notable changes yet._

### Security

- _No notable changes yet._

## [0.4.0] - 2026-04-12

### Changed

- Breaking: replaced the local Postgres/pgvector runtime with a SQLite-only runtime built on FTS5 and `sqlite-vec`, changed config from `database.dsn` to `database.path`, and made rebuild-from-Confluence the supported reindex/migration path. ([#3](https://github.com/mbIkola/viki/pull/3))

## [0.3.0] - 2026-04-12

### Added

- Multi-root sync (`confluence.parent_ids`) with CLI/API overrides.
- Scope semantics: `full` (config roots) vs `partial` (manual roots).
- Tests for root resolution and partial delete-suppression behavior.

### Changed

- Breaking config change: removed `default_parent_id`, replaced by `confluence.parent_ids`.
- MCP implementation version now comes from build-time injected version metadata.

### Fixed

- Partial sync no longer produces false `deleted` events for pages outside manually selected roots.

## [0.2.0] - 2026-04-07

### Added

- MCP facade (`cmd/mcp`) with tools: `search`, `ask`, `get_tree`, `what_changed`.
- Structured change intelligence (`what_changed`) with reasons and optional excerpts.
- MCP smoke test script and local operational tooling for runtime bootstrap.

### Changed

- Local runtime flow documented as Postgres+pgvector with host-native Ollama.

## [0.1.0] - 2026-04-07

### Added

- Initial `confluence-replica` service: bootstrap/sync/digest and internal HTTP API.
- Postgres storage schema with versions, chunks, embeddings, sync runs, and digests.
- Hybrid retrieval (FTS + embeddings), deterministic RAG with citations, and daily digest generation.
