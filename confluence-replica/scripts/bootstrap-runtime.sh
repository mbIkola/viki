#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CONFIG_PATH="${ROOT_DIR}/config/config.yaml"
SKIP_OLLAMA=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --config)
      CONFIG_PATH="$2"
      shift 2
      ;;
    --no-ollama)
      SKIP_OLLAMA=1
      shift
      ;;
    *)
      echo "Unknown argument: $1" >&2
      echo "Usage: $0 [--config path] [--no-ollama]" >&2
      exit 1
      ;;
  esac
done

log() {
  echo "[bootstrap-runtime] $*"
}

if [[ -f "${ROOT_DIR}/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "${ROOT_DIR}/.env"
  set +a
fi

POSTGRES_DB="${POSTGRES_DB:-confluence_replica}"
POSTGRES_USER="${POSTGRES_USER:-postgres}"
PG_DATA_DIR="${PG_DATA_DIR:-${HOME}/.local/viki/confluence/postgres-data}"

mkdir -p "${PG_DATA_DIR}"
log "Using postgres data dir: ${PG_DATA_DIR}"

log "Starting Postgres (docker compose)..."
(
  cd "${ROOT_DIR}"
  docker compose up -d postgres
)

ready=0
for _ in {1..40}; do
  if (
    cd "${ROOT_DIR}" && \
    docker compose exec -T postgres pg_isready -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" >/dev/null 2>&1
  ); then
    ready=1
    break
  fi
  sleep 1
done
if [[ "${ready}" != "1" ]]; then
  log "Postgres did not become ready in time"
  exit 1
fi
log "Postgres is ready"

if [[ "${SKIP_OLLAMA}" == "1" ]]; then
  log "Skipping Ollama startup (--no-ollama)"
  exit 0
fi

read_embeddings_key() {
  local key="$1"
  local file="$2"
  [[ -f "${file}" ]] || return 0

  awk -v key="${key}" '
    /^embeddings:/ { in_block=1; next }
    in_block && /^[^[:space:]]/ { in_block=0 }
    in_block {
      if ($1 == key ":") {
        val=$2
        gsub(/"/, "", val)
        print val
        exit
      }
    }
  ' "${file}"
}

EMBED_PROVIDER="${EMBEDDINGS_PROVIDER:-$(read_embeddings_key provider "${CONFIG_PATH}")}" 
if [[ -z "${EMBED_PROVIDER}" ]]; then
  EMBED_PROVIDER="ollama"
fi

OLLAMA_BASE_URL="${OLLAMA_BASE_URL:-$(read_embeddings_key base_url "${CONFIG_PATH}")}" 
if [[ -z "${OLLAMA_BASE_URL}" ]]; then
  OLLAMA_BASE_URL="http://127.0.0.1:11434"
fi

OLLAMA_EMBED_MODEL="${OLLAMA_EMBED_MODEL:-$(read_embeddings_key model "${CONFIG_PATH}")}" 

if [[ "${EMBED_PROVIDER}" != "ollama" ]]; then
  log "Embeddings provider is '${EMBED_PROVIDER}', skipping Ollama startup"
  exit 0
fi

if [[ -z "${OLLAMA_EMBED_MODEL}" ]]; then
  log "Embeddings model not set, skipping model pull"
  exit 0
fi

if ! curl -fsS "${OLLAMA_BASE_URL}/api/version" >/dev/null 2>&1; then
  if ! command -v ollama >/dev/null 2>&1; then
    log "Ollama is not running and 'ollama' binary was not found"
    exit 1
  fi

  log "Starting Ollama server..."
  nohup ollama serve >"${ROOT_DIR}/.ollama-serve.log" 2>&1 &

  up=0
  for _ in {1..30}; do
    if curl -fsS "${OLLAMA_BASE_URL}/api/version" >/dev/null 2>&1; then
      up=1
      break
    fi
    sleep 1
  done
  if [[ "${up}" != "1" ]]; then
    log "Ollama server did not become reachable at ${OLLAMA_BASE_URL}"
    exit 1
  fi
fi
log "Ollama is reachable at ${OLLAMA_BASE_URL}"

status_file="$(mktemp)"
status_code="$({
  curl -sS -o "${status_file}" -w "%{http_code}" \
    -X POST "${OLLAMA_BASE_URL}/api/show" \
    -H 'Content-Type: application/json' \
    -d "{\"model\":\"${OLLAMA_EMBED_MODEL}\"}"
} || true)"

if [[ "${status_code}" != "200" ]]; then
  if ! command -v ollama >/dev/null 2>&1; then
    log "Model '${OLLAMA_EMBED_MODEL}' is missing and 'ollama' binary not found"
    rm -f "${status_file}"
    exit 1
  fi

  log "Model '${OLLAMA_EMBED_MODEL}' is missing, pulling..."
  ollama pull "${OLLAMA_EMBED_MODEL}"
else
  log "Model '${OLLAMA_EMBED_MODEL}' is already available"
fi

rm -f "${status_file}"
log "Runtime bootstrap complete"
