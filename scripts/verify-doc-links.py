#!/usr/bin/env python3
from __future__ import annotations

import pathlib
import re
import sys


ROOT = pathlib.Path(__file__).resolve().parents[1]
DOC_PATHS = [
    ROOT / "AGENTS.md",
    ROOT / "docs" / "agent-governance" / "coverage-matrix.md",
    *sorted((ROOT / ".cursor" / "skills").glob("*/SKILL.md")),
    *sorted((ROOT / ".cursor" / "rules").glob("*.mdc")),
]
PATH_PATTERN = re.compile(
    r"(?<![\w./-])(?P<path>(?:AGENTS\.md|Makefile|docs/[\w./-]+|\.cursor/[\w./-]+|\.build/[\w./-]+|scripts/[\w./-]+)(?:\.(?:md|mdc|yaml|yml|json|py|sh|tmpl|neg))?)"
)
IGNORED_PREFIXES = ("https://", "http://")


def referenced_paths(text: str) -> set[str]:
    paths: set[str] = set()
    for match in PATH_PATTERN.finditer(text):
        path = match.group("path").rstrip(".,，。)：)")
        if not path.startswith(IGNORED_PREFIXES) and "*" not in path and "|" not in path and not path.endswith("-"):
            paths.add(path)
    return paths


def main() -> int:
    errors: list[str] = []
    for owner in DOC_PATHS:
        if not owner.exists():
            continue
        for rel_path in sorted(referenced_paths(owner.read_text())):
            target = ROOT / rel_path
            if not target.exists():
                errors.append(f"{owner.relative_to(ROOT)}: referenced path does not exist: {rel_path}")

    if errors:
        for error in errors:
            print(error, file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
