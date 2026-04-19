#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT_PATH="$ROOT_DIR/scripts/rotate-atlassian-pats.sh"

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT
LOG_FILE="$TMP_DIR/calls.log"
MOCK_BIN="$TMP_DIR/bin"
mkdir -p "$MOCK_BIN"

cat > "$MOCK_BIN/open" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
echo "open:$*" >> "$LOG_FILE"
MOCK

cat > "$MOCK_BIN/security" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
echo "security:$*" >> "$LOG_FILE"
MOCK

cat > "$MOCK_BIN/osascript" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
echo "osascript:$*" >> "$LOG_FILE"
MOCK

chmod +x "$MOCK_BIN/open" "$MOCK_BIN/security" "$MOCK_BIN/osascript"

run_test() {
  PATH="$MOCK_BIN:$PATH" \
  LOG_FILE="$LOG_FILE" \
  KEYCHAIN_ACCOUNT="tester" \
  JIRA_KEYCHAIN_SERVICE="codex_jira_pat" \
  CONFLUENCE_KEYCHAIN_SERVICE="codex_confluence_pat" \
  REMINDER_LIST="Reminders" \
  REMIND_AFTER_DAYS="4" \
  bash "$SCRIPT_PATH" <<< $'jira-token-123\nconfluence-token-456\n'
}

run_test_with_input() {
  local input="$1"
  PATH="$MOCK_BIN:$PATH" \
  LOG_FILE="$LOG_FILE" \
  KEYCHAIN_ACCOUNT="tester" \
  JIRA_KEYCHAIN_SERVICE="codex_jira_pat" \
  CONFLUENCE_KEYCHAIN_SERVICE="codex_confluence_pat" \
  REMINDER_LIST="Reminders" \
  REMIND_AFTER_DAYS="4" \
  bash "$SCRIPT_PATH" <<< "$input"
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

run_test

assert_contains "open:https://gbujira.oraclecorp.com/secure/ViewProfile.jspa?selectedTab=com.atlassian.pats.pats-plugin:jira-user-personal-access-tokens"
assert_contains "open:https://gbuconfluence.oraclecorp.com/wiki/users/viewmyprofile.action?selectedTab=com.atlassian.pats.pats-plugin:personal-access-tokens"
assert_contains "security:add-generic-password -U -s codex_jira_pat -a tester -w jira-token-123"
assert_contains "security:add-generic-password -U -s codex_confluence_pat -a tester -w confluence-token-456"
assert_contains "osascript:- Rotate Jira/Confluence PATs"

set +e
error_output="$(run_test_with_input $'\nconfluence-token-456\n' 2>&1)"
exit_code="$?"
set -e

if [[ "$exit_code" -eq 0 ]]; then
  echo "expected failure when Jira token is empty"
  exit 1
fi
if ! grep -F "Jira PAT cannot be empty" <<< "$error_output" >/dev/null; then
  echo "expected empty-token error message"
  echo "$error_output"
  exit 1
fi

echo "ok"
