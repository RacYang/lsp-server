#!/usr/bin/env python3
from __future__ import annotations

import argparse
import pathlib
import re
import sys


ROOT = pathlib.Path(__file__).resolve().parents[1]


def read(path: pathlib.Path) -> str:
    return path.read_text()


def check_naming(path: pathlib.Path) -> list[str]:
    text = read(path)
    errors: list[str] = []
    for match in re.finditer(r"\b(type|func|var|const)\s+([A-Z][A-Za-z0-9_]*)", text):
        name = match.group(2)
        if "_" in name:
            errors.append(f"{path}: exported identifier must not contain underscore: {name}")
    return errors


def check_package_doc(path: pathlib.Path) -> list[str]:
    text = read(path)
    if re.search(r"^// Package [a-z][a-z0-9_]* .*[一-龥]", text, re.MULTILINE):
        return []
    return [f"{path}: missing Chinese package comment"]


def check_generated_marker(path: pathlib.Path) -> list[str]:
    text = read(path)
    if "Code generated" in text and "DO NOT EDIT." in text:
        return []
    return [f"{path}: missing generated marker"]


def check_test_layout(path: pathlib.Path) -> list[str]:
    text = read(path)
    if "tests := []struct" in text:
        return []
    return [f"{path}: table tests must use tests := []struct"]


def check_file_header(path: pathlib.Path) -> list[str]:
    text = read(path)
    if path.suffix == ".sh" and text.startswith("#!/usr/bin/env bash") and "set -euo pipefail" in text:
        return []
    if path.suffix == ".py" and text.startswith("#!/usr/bin/env python3"):
        return []
    return [f"{path}: missing standard script header"]


def check_rule_shape(path: pathlib.Path) -> list[str]:
    text = read(path)
    required = ("**ADR**", "**Enforcer**", "**负例**")
    errors = [f"{path}: missing {token}" for token in required if token not in text]
    if "description:" not in text:
        errors.append(f"{path}: missing description in frontmatter")
    return errors


CHECKS = {
    "naming": check_naming,
    "package-doc": check_package_doc,
    "generated-marker": check_generated_marker,
    "test-layout": check_test_layout,
    "file-header": check_file_header,
    "rule-shape": check_rule_shape,
}


def candidate_files(check: str) -> list[pathlib.Path]:
    if check == "package-doc":
        return [p for p in (ROOT / "internal").glob("**/doc.go") if "gen" not in p.parts]
    if check == "generated-marker":
        return [p for p in ROOT.glob("**/*.pb.go") if "api" in p.parts and "gen" in p.parts]
    if check == "test-layout":
        return [p for p in ROOT.glob("**/*_test.go") if ".build" not in p.parts]
    if check == "file-header":
        return sorted(ROOT.glob("scripts/*.sh")) + sorted(ROOT.glob("scripts/*.py"))
    if check == "rule-shape":
        return sorted((ROOT / ".claude" / "rules").glob("*.md")) + sorted(
            (ROOT / ".codex" / "skills" / "lsp-server-governance" / "references" / "rules").glob("*.md")
        )
    return [p for p in ROOT.glob("**/*.go") if "gen" not in p.parts and ".build" not in p.parts]


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--check", required=True, choices=sorted(CHECKS))
    parser.add_argument("--file")
    args = parser.parse_args()

    files = [pathlib.Path(args.file)] if args.file else candidate_files(args.check)
    errors: list[str] = []
    for file in files:
        if file.exists():
            errors.extend(CHECKS[args.check](file))
    if errors:
        for error in errors:
            print(error, file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
