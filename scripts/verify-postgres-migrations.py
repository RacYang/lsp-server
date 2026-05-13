#!/usr/bin/env python3
"""校验 PostgreSQL 迁移文件命名与幂等 DDL 约束。"""

from __future__ import annotations

import re
import sys
from pathlib import Path


import argparse

ROOT = Path(__file__).resolve().parents[1]
MIGRATIONS = ROOT / "internal" / "store" / "postgres" / "migrations"
NAME = re.compile(r"^\d{3}_[a-z0-9_]+\.sql$")
DANGEROUS = re.compile(r"\b(DROP\s+TABLE|DROP\s+COLUMN|TRUNCATE|ALTER\s+TYPE)\b", re.IGNORECASE)
CREATE_WITHOUT_IF = re.compile(r"\bCREATE\s+(?:UNIQUE\s+)?(?:TABLE|INDEX)\b(?!\s+IF\s+NOT\s+EXISTS)", re.IGNORECASE)
ADD_COLUMN_WITHOUT_IF = re.compile(r"\bADD\s+COLUMN\b(?!\s+IF\s+NOT\s+EXISTS)", re.IGNORECASE)


def strip_comments(sql: str) -> str:
    lines = []
    for line in sql.splitlines():
        lines.append(line.split("--", 1)[0])
    return "\n".join(lines)


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--dir", type=Path, help="负例模式：校验指定的迁移目录")
    args = ap.parse_args()

    mig_dir = args.dir if args.dir else MIGRATIONS
    errors: list[str] = []
    files = sorted(mig_dir.glob("*.sql"))
    if not files:
        errors.append(f"{mig_dir}: 缺少 PostgreSQL 迁移文件")
    expected = 1
    for path in files:
        rel = path.relative_to(ROOT).as_posix()
        if not NAME.match(path.name):
            errors.append(f"{rel}: 迁移文件名必须形如 001_descriptive_name.sql")
        prefix = path.name.split("_", 1)[0]
        if prefix.isdigit() and int(prefix) != expected:
            errors.append(f"{rel}: 迁移编号应连续，期望 {expected:03d}")
        expected += 1
        sql = strip_comments(path.read_text(encoding="utf-8"))
        if DANGEROUS.search(sql):
            errors.append(f"{rel}: 默认禁止破坏性 DDL；如确需执行请先补 ADR")
        if CREATE_WITHOUT_IF.search(sql):
            errors.append(f"{rel}: CREATE TABLE/INDEX 必须使用 IF NOT EXISTS")
        if ADD_COLUMN_WITHOUT_IF.search(sql):
            errors.append(f"{rel}: ADD COLUMN 必须使用 IF NOT EXISTS")
    if errors:
        print("\n".join(errors), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
