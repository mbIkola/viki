#!/usr/bin/env bash
set -euo pipefail

KEYCHAIN_SERVICE="${CONFLUENCE_PAT_KEYCHAIN_SERVICE:-codex_confluence_pat}"
CONFLUENCE_URL="${CONFLUENCE_URL:-https://gbuconfluence.oraclecorp.com/}"
CONFLUENCE_SPACES_FILTER="${CONFLUENCE_SPACES_FILTER:-UGBUPD}"
UV_CACHE_DIR="${UV_CACHE_DIR:-/tmp/codex-uv-cache}"
UV_TOOL_DIR="${UV_TOOL_DIR:-/tmp/codex-uv-tools}"
XDG_DATA_HOME="${XDG_DATA_HOME:-/tmp/codex-xdg-data}"
MCP_ATLASSIAN_BIN="${MCP_ATLASSIAN_BIN:-}"

mkdir -p "$UV_CACHE_DIR"
mkdir -p "$UV_TOOL_DIR"
mkdir -p "$XDG_DATA_HOME"
export UV_CACHE_DIR
export UV_TOOL_DIR
export XDG_DATA_HOME

run_mcp_atlassian() {
  if [[ -n "$MCP_ATLASSIAN_BIN" && -x "$MCP_ATLASSIAN_BIN" ]]; then
    exec "$MCP_ATLASSIAN_BIN" "$@"
  fi
  if command -v mcp-atlassian >/dev/null 2>&1; then
    exec "$(command -v mcp-atlassian)" "$@"
  fi
  if ! command -v uvx >/dev/null 2>&1; then
    echo "mcp-atlassian not found (brew/global), and uvx is unavailable." >&2
    exit 1
  fi
  exec uvx mcp-atlassian "$@"
}

for arg in "$@"; do
  if [[ "$arg" == "--help" || "$arg" == "--version" ]]; then
    run_mcp_atlassian "$@"
  fi
done

if [[ -n "${CONFLUENCE_PAT:-}" ]]; then
  TOKEN="${CONFLUENCE_PAT}"
else
  TOKEN="$(security find-generic-password -a "$USER" -s "$KEYCHAIN_SERVICE" -w)"
fi

if [[ -z "${TOKEN}" ]]; then
  echo "Confluence PAT not found. Set CONFLUENCE_PAT or store it in Keychain service '${KEYCHAIN_SERVICE}'." >&2
  exit 1
fi

run_mcp_atlassian \
  --transport stdio \
  --read-only \
  --confluence-url "$CONFLUENCE_URL" \
  --confluence-personal-token "$TOKEN" \
  --confluence-spaces-filter "$CONFLUENCE_SPACES_FILTER" \
  "$@"
