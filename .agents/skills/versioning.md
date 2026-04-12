# Versioning Policy (Monorepo)

This repository uses **per-project namespaced tags** for `confluence-replica`:

- `confluence-replica/vX.Y.Z`

## Source of truth

- Release version is defined by git tag.
- `confluence-replica/CHANGELOG.md` is maintained by humans in `Unreleased`.
- Build metadata (`Version`, `Commit`, `BuildDate`) is injected at build time via `-ldflags`.

## Changelog workflow

1. During development, update `## [Unreleased]` in `confluence-replica/CHANGELOG.md`.
2. Keep entries under `Added / Changed / Fixed / Removed / Security`.
3. Do not manually create release headings during regular PR flow.

## Release workflow

Use GitHub Actions manual workflow:

- Workflow: `confluence-replica-release`
- Input: `version` as `X.Y.Z`
- Preconditions:
  - run from `main`
  - `Unreleased` has at least one non-placeholder bullet
  - tag `confluence-replica/vX.Y.Z` does not already exist

What it does:

1. Validates version and changelog.
2. Promotes `Unreleased` to `## [X.Y.Z] - YYYY-MM-DD`.
3. Recreates empty `Unreleased` template.
4. Commits changelog update to `main`.
5. Creates and pushes tag `confluence-replica/vX.Y.Z`.
6. Publishes GitHub Release with notes from changelog section.

## CI expectations

- `confluence-replica-ci` runs tests (`go test ./...`) and changelog structure checks.
- Any change to release process files should keep both CI and release workflow valid.

## Rollback / hotfix

- Hotfix is a normal patch release: update `Unreleased`, run manual release with next patch version.
- Do not reuse or retag existing versions.
