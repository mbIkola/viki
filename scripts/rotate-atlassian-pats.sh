#!/usr/bin/env bash
set -euo pipefail

JIRA_PAT_URL="${JIRA_PAT_URL:-https://gbujira.oraclecorp.com/secure/ViewProfile.jspa?selectedTab=com.atlassian.pats.pats-plugin:jira-user-personal-access-tokens}"
CONFLUENCE_PAT_URL="${CONFLUENCE_PAT_URL:-https://gbuconfluence.oraclecorp.com/wiki/users/viewmyprofile.action?selectedTab=com.atlassian.pats.pats-plugin:personal-access-tokens}"
JIRA_KEYCHAIN_SERVICE="${JIRA_KEYCHAIN_SERVICE:-codex_jira_pat}"
CONFLUENCE_KEYCHAIN_SERVICE="${CONFLUENCE_KEYCHAIN_SERVICE:-codex_confluence_pat}"
KEYCHAIN_ACCOUNT="${KEYCHAIN_ACCOUNT:-$(whoami)}"
REMINDER_LIST="${REMINDER_LIST:-Reminders}"
REMINDER_TITLE="${REMINDER_TITLE:-Rotate Jira/Confluence PATs}"
REMIND_AFTER_DAYS="${REMIND_AFTER_DAYS:-4}"

OPEN_CMD="${OPEN_CMD:-open}"
SECURITY_CMD="${SECURITY_CMD:-security}"
OSASCRIPT_CMD="${OSASCRIPT_CMD:-osascript}"

if ! [[ "$REMIND_AFTER_DAYS" =~ ^[0-9]+$ ]]; then
  echo "REMIND_AFTER_DAYS must be a non-negative integer" >&2
  exit 1
fi

for required in "$OPEN_CMD" "$SECURITY_CMD" "$OSASCRIPT_CMD"; do
  if ! command -v "$required" >/dev/null 2>&1; then
    echo "Required command not found: $required" >&2
    exit 1
  fi
done

script_path="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/$(basename "${BASH_SOURCE[0]}")"

printf 'Opening PAT pages in browser...\n'
"$OPEN_CMD" "$JIRA_PAT_URL"
"$OPEN_CMD" "$CONFLUENCE_PAT_URL"

read -r -s -p "Enter new Jira PAT: " jira_token
echo
if [[ -z "$jira_token" ]]; then
  echo "Jira PAT cannot be empty" >&2
  exit 1
fi

read -r -s -p "Enter new Confluence PAT: " confluence_token
echo
if [[ -z "$confluence_token" ]]; then
  echo "Confluence PAT cannot be empty" >&2
  exit 1
fi

"$SECURITY_CMD" add-generic-password -U -s "$JIRA_KEYCHAIN_SERVICE" -a "$KEYCHAIN_ACCOUNT" -w "$jira_token"
"$SECURITY_CMD" add-generic-password -U -s "$CONFLUENCE_KEYCHAIN_SERVICE" -a "$KEYCHAIN_ACCOUNT" -w "$confluence_token"

reminder_body=$(cat <<BODY
Manual PAT rotation (2FA required).

1) Open Jira PAT page:
$JIRA_PAT_URL
2) Open Confluence PAT page:
$CONFLUENCE_PAT_URL
3) Run this command:
$script_path

Keychain services:
- $JIRA_KEYCHAIN_SERVICE
- $CONFLUENCE_KEYCHAIN_SERVICE
BODY
)

"$OSASCRIPT_CMD" - "$REMINDER_TITLE" "$reminder_body" "$REMINDER_LIST" "$REMIND_AFTER_DAYS" <<'APPLESCRIPT'
on run argv
  set reminderTitle to item 1 of argv
  set reminderBody to item 2 of argv
  set reminderListName to item 3 of argv
  set daysAhead to (item 4 of argv) as integer
  set dueDateValue to (current date) + (daysAhead * days)

  tell application "Reminders"
    if not (exists list reminderListName) then
      make new list with properties {name:reminderListName}
    end if
    set targetList to list reminderListName
    make new reminder at end of reminders of targetList with properties {name:reminderTitle, body:reminderBody, due date:dueDateValue}
  end tell
end run
APPLESCRIPT

echo "Updated keychain tokens for account: $KEYCHAIN_ACCOUNT"
echo "  Jira: keychain://$JIRA_KEYCHAIN_SERVICE?account=$KEYCHAIN_ACCOUNT"
echo "  Confluence: keychain://$CONFLUENCE_KEYCHAIN_SERVICE?account=$KEYCHAIN_ACCOUNT"
echo "Reminder created in list '$REMINDER_LIST' for +$REMIND_AFTER_DAYS day(s)."
