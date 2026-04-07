#!/usr/bin/env python3
import re
import sys


CHANGE_PATTERNS = [
    re.compile(r"(что\s+изменил[ао]с[ья]|что\s+нового).*(confluence|конфлю|вики)", re.IGNORECASE),
    re.compile(r"(что\s+писали|пока\s+меня\s+не\s+было).*(confluence|конфлю|вики)", re.IGNORECASE),
    re.compile(r"(what\s+changed|what'?s\s+new|updates?\s+in|catch\s+me\s+up).*(confluence|wiki)", re.IGNORECASE),
]


def is_trigger(text: str) -> bool:
    return any(p.search(text) for p in CHANGE_PATTERNS)


def main() -> int:
    positives = [
        "что изменилось в конфлюенсе пока меня не было",
        "что нового в confluence по проекту",
        "what changed in confluence this week",
        "catch me up on wiki updates",
    ]
    negatives = [
        "найди onboarding page в confluence",
        "show me architecture page",
    ]
    for q in positives:
        if not is_trigger(q):
            print(f"FAIL: expected trigger for: {q}", file=sys.stderr)
            return 1
    for q in negatives:
        if is_trigger(q):
            print(f"FAIL: unexpected trigger for: {q}", file=sys.stderr)
            return 1
    print("PASS: confluence-change skill trigger smoke")
    return 0


if __name__ == "__main__":
    sys.exit(main())
