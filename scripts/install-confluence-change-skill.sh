#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SRC_DIR="$ROOT_DIR/skills/confluence-change-intelligence"
CODEX_HOME="${CODEX_HOME:-$HOME/.codex}"
DST_DIR="$CODEX_HOME/skills/confluence-change-intelligence"

if [[ ! -f "$SRC_DIR/SKILL.md" ]]; then
  echo "missing source skill at $SRC_DIR/SKILL.md" >&2
  exit 1
fi

mkdir -p "$CODEX_HOME/skills"
rm -rf "$DST_DIR"
cp -R "$SRC_DIR" "$DST_DIR"

echo "installed skill: $DST_DIR"
