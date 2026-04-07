#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

MCP_BIN="${CONFLUENCE_REPLICA_MCP_BIN:-${ROOT_DIR}/bin/mcp}"
CONFIG_PATH="${CONF_REPLICA_CONFIG:-${ROOT_DIR}/config/config.yaml}"
CODEX_HOME_DIR="${CODEX_HOME:-${HOME}/.codex}"
LOG_DIR="${CODEX_HOME_DIR}/log"
LOG_FILE="${CONFLUENCE_REPLICA_MCP_LOG:-${LOG_DIR}/confluence-replica-mcp.log}"

if ! mkdir -p "${LOG_DIR}" 2>/dev/null; then
  LOG_FILE="/tmp/confluence-replica-mcp.log"
fi

if [[ ! -x "${MCP_BIN}" ]]; then
  echo "confluence-replica mcp binary not executable: ${MCP_BIN}" >>"${LOG_FILE}" 2>/dev/null || true
  exit 1
fi

if {
  echo "-----"
  echo "[$(date '+%Y-%m-%d %H:%M:%S')] starting confluence-replica mcp"
  echo "bin=${MCP_BIN}"
  echo "config=${CONFIG_PATH}"
} >>"${LOG_FILE}" 2>/dev/null; then
  exec "${MCP_BIN}" --config "${CONFIG_PATH}" "$@" 2>>"${LOG_FILE}"
fi

exec "${MCP_BIN}" --config "${CONFIG_PATH}" "$@"
