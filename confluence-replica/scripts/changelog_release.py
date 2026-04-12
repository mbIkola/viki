#!/usr/bin/env python3
import argparse
import datetime as dt
import pathlib
import re
import sys

UNRELEASED = "Unreleased"
REQUIRED_UNRELEASED_SUBSECTIONS = ["Added", "Changed", "Fixed", "Removed", "Security"]
PLACEHOLDER_LINES = {
    "- _No notable changes yet._",
    "- none",
    "- None",
}


def load_lines(path: pathlib.Path) -> list[str]:
    if not path.exists():
        raise ValueError(f"changelog not found: {path}")
    return path.read_text(encoding="utf-8").splitlines()


def write_lines(path: pathlib.Path, lines: list[str]) -> None:
    path.write_text("\n".join(lines).rstrip() + "\n", encoding="utf-8")


def section_bounds(lines: list[str], name: str) -> tuple[int, int]:
    heading = f"## [{name}]"
    starts = [i for i, line in enumerate(lines) if line.startswith(heading)]
    if not starts:
        raise ValueError(f"missing section: {heading}")
    start = starts[0]
    end = len(lines)
    for i in range(start + 1, len(lines)):
        if lines[i].startswith("## ["):
            end = i
            break
    return start, end


def has_unreleased_changes(lines: list[str]) -> bool:
    start, end = section_bounds(lines, UNRELEASED)
    body = lines[start + 1 : end]
    for line in body:
        s = line.strip()
        if not s.startswith("- "):
            continue
        if s in PLACEHOLDER_LINES:
            continue
        return True
    return False


def validate_changelog(lines: list[str]) -> None:
    section_bounds(lines, UNRELEASED)
    start, end = section_bounds(lines, UNRELEASED)
    body = lines[start + 1 : end]
    found = [line.strip()[4:] for line in body if line.strip().startswith("### ")]
    missing = [name for name in REQUIRED_UNRELEASED_SUBSECTIONS if name not in found]
    if missing:
        raise ValueError(f"Unreleased section missing subsections: {', '.join(missing)}")


def new_unreleased_template() -> list[str]:
    lines: list[str] = ["## [Unreleased]", ""]
    for idx, name in enumerate(REQUIRED_UNRELEASED_SUBSECTIONS):
        lines.extend([f"### {name}", "", "- _No notable changes yet._"])
        if idx < len(REQUIRED_UNRELEASED_SUBSECTIONS) - 1:
            lines.append("")
    return lines


def release(lines: list[str], version: str, date_text: str) -> list[str]:
    if not re.fullmatch(r"\d+\.\d+\.\d+", version):
        raise ValueError("version must be X.Y.Z")
    section_name = version
    try:
        section_bounds(lines, section_name)
    except ValueError:
        pass
    else:
        raise ValueError(f"release section already exists: {section_name}")

    if not has_unreleased_changes(lines):
        raise ValueError("Unreleased has no releasable entries")

    start, end = section_bounds(lines, UNRELEASED)
    unreleased_body = trim_blank(lines[start + 1 : end])
    release_heading = f"## [{version}] - {date_text}"

    rebuilt = []
    rebuilt.extend(lines[:start])
    if rebuilt and rebuilt[-1] != "":
        rebuilt.append("")
    rebuilt.extend(new_unreleased_template())
    rebuilt.append("")
    rebuilt.append(release_heading)
    rebuilt.append("")
    rebuilt.extend(unreleased_body)
    rebuilt.append("")
    rebuilt.extend(lines[end:])
    return tidy_blank_lines(rebuilt)


def extract_release(lines: list[str], version: str) -> str:
    start, end = section_bounds(lines, version)
    chunk = lines[start:end]
    return "\n".join(chunk).strip() + "\n"


def trim_blank(lines: list[str]) -> list[str]:
    start = 0
    end = len(lines)
    while start < end and lines[start].strip() == "":
        start += 1
    while end > start and lines[end - 1].strip() == "":
        end -= 1
    return lines[start:end]


def tidy_blank_lines(lines: list[str]) -> list[str]:
    out: list[str] = []
    prev_blank = False
    for line in lines:
        blank = line.strip() == ""
        if blank and prev_blank:
            continue
        out.append(line)
        prev_blank = blank
    return out


def main() -> int:
    parser = argparse.ArgumentParser(description="changelog helper for confluence-replica")
    parser.add_argument("--file", default="confluence-replica/CHANGELOG.md", help="path to changelog")
    sub = parser.add_subparsers(dest="cmd", required=True)
    sub.add_parser("check")
    sub.add_parser("has-unreleased")
    rel = sub.add_parser("release")
    rel.add_argument("--version", required=True)
    rel.add_argument("--date", default=dt.date.today().isoformat())
    ext = sub.add_parser("extract")
    ext.add_argument("--version", required=True)
    args = parser.parse_args()

    path = pathlib.Path(args.file)
    lines = load_lines(path)

    try:
        if args.cmd == "check":
            validate_changelog(lines)
            return 0
        if args.cmd == "has-unreleased":
            return 0 if has_unreleased_changes(lines) else 1
        if args.cmd == "release":
            validate_changelog(lines)
            updated = release(lines, args.version, args.date)
            write_lines(path, updated)
            return 0
        if args.cmd == "extract":
            sys.stdout.write(extract_release(lines, args.version))
            return 0
    except ValueError as exc:
        print(f"[error] {exc}", file=sys.stderr)
        return 2

    return 1


if __name__ == "__main__":
    raise SystemExit(main())
