#!/usr/bin/env python3
"""校验 Redis 键集中构造，并保持 lsp: 前缀。"""

from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path

from lang_verify_common import ROOT, load_section, posix_rel


REDIS_CALL = re.compile(
    r"\.(?:Get|Set|SetNX|HSet|HGet|Del|Expire|Exists|Publish|Subscribe)\s*\([^,\n]+,\s*\"([^\"]+)\"",
)
FMT_KEY = re.compile(r"fmt\.Sprintf\(\"([^\"]+)\"")


def check_source(path: Path, key_prefix: str, constructor_rels: set[str]) -> list[str]:
    rel = posix_rel(path.resolve())
    source = path.read_text(encoding="utf-8")
    errors: list[str] = []
    if rel in constructor_rels:
        for prefix in FMT_KEY.findall(source):
            if not prefix.startswith(key_prefix):
                errors.append(f"{rel}: Redis 键构造器前缀必须以 {key_prefix!r} 开头: {prefix!r}")
        return errors
    for key in REDIS_CALL.findall(source):
        errors.append(f"{rel}: Redis 操作不得直接使用字面量键 {key!r}，请通过 keys.go 构造")
    return errors


def target_files(single_file: Path | None, constructor_rels: set[str]) -> list[Path]:
    if single_file:
        return [single_file.resolve()]
    files = sorted(
        p
        for p in (ROOT / "internal" / "store" / "redis").glob("**/*.go")
        if not p.name.endswith("_test.go")
    )
    constructor_files = [ROOT / rel for rel in constructor_rels]
    return [p for p in files if p.is_file()] + [p for p in constructor_files if p.is_file()]


def main() -> int:
    parser = argparse.ArgumentParser(description="校验 Redis 键名与集中构造")
    parser.add_argument("--file", type=Path, help="仅校验单个文件（负例模式）")
    args = parser.parse_args()

    config = load_section("redis")
    key_prefix = str(config["key_prefix"])
    constructor_rels = {str(p) for p in config["key_constructors"]}
    errors: list[str] = []
    for path in target_files(args.file, constructor_rels):
        errors.extend(check_source(path, key_prefix, constructor_rels))
    if errors:
        print("\n".join(errors), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
