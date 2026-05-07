#!/usr/bin/env python3
"""校验 nolint 豁免必须可审计。"""

from __future__ import annotations

import argparse
import json
import pathlib
import re
import subprocess
import sys

ROOT = pathlib.Path(__file__).resolve().parents[1]
NOLINT_RE = re.compile(r"//nolint(?::(?P<linters>[a-z0-9_,]+))?(?P<tail>.*)$")
GENERATED_DIR_PART = "/gen/"


def enabled_linters() -> set[str]:
    raw = subprocess.check_output(["yq", "-o=json", ".lint.golangci.enable", str(ROOT / ".build/config.yaml")], text=True)
    return set(json.loads(raw))


def go_files(target: pathlib.Path | None) -> list[pathlib.Path]:
    if target is not None:
        return [target]
    return [
        path
        for path in ROOT.rglob("*.go")
        if ".git" not in path.parts and ".build" not in path.parts and GENERATED_DIR_PART not in path.as_posix()
    ]


def validate_file(path: pathlib.Path, enabled: set[str]) -> list[str]:
    errors: list[str] = []
    try:
        lines = path.read_text(encoding="utf-8").splitlines()
    except UnicodeDecodeError:
        return errors
    for line_no, line in enumerate(lines, start=1):
        match = NOLINT_RE.search(line)
        if not match:
            continue
        raw_linters = match.group("linters")
        if not raw_linters:
            errors.append(f"{path}:{line_no}: nolint 必须显式指定 linter")
            continue
        linters = [item.strip() for item in raw_linters.split(",") if item.strip()]
        unknown = [item for item in linters if item not in enabled]
        if unknown:
            errors.append(f"{path}:{line_no}: nolint 指向未启用 linter: {', '.join(unknown)}")
        tail = match.group("tail")
        reason = ""
        if "//" in tail:
            reason = tail.split("//", 1)[1].strip()
        if not reason:
            errors.append(f"{path}:{line_no}: nolint 必须在同一行写明原因")
    return errors


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--file", type=pathlib.Path)
    args = parser.parse_args()

    enabled = enabled_linters()
    errors: list[str] = []
    for path in go_files(args.file):
        errors.extend(validate_file(path, enabled))
    if errors:
        print("\n".join(errors), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
