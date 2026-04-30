#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT
LOG_FILE="$TMP_DIR/calls.log"
MOCK_BIN="$TMP_DIR/bin"
mkdir -p "$MOCK_BIN"

cat > "$MOCK_BIN/security" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
echo "security:$*" >> "$LOG_FILE"
MOCK

chmod +x "$MOCK_BIN/security"

run_update_token() {
  PATH="$MOCK_BIN:$PATH" \
  LOG_FILE="$LOG_FILE" \
  make -C "$ROOT_DIR/confluence-replica" update-token \
    KEYCHAIN_ACCOUNT="tester" \
    JIRA_KEYCHAIN_SERVICE="codex_jira_pat" \
    KEYCHAIN_SERVICE="codex_confluence_pat" \
    <<< $'jira-token-123\nconfluence-token-456\n'
}

assert_contains() {
  local needle="$1"
  if ! grep -F "$needle" "$LOG_FILE" >/dev/null; then
    echo "expected to find: $needle"
    echo "actual log:"
    cat "$LOG_FILE"
    exit 1
  fi
}

run_update_token >/dev/null

assert_contains "security:add-generic-password -U -s codex_jira_pat -a tester -w jira-token-123"
assert_contains "security:add-generic-password -U -s codex_confluence_pat -a tester -w confluence-token-456"

echo "ok"
