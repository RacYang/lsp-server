#!/usr/bin/env python3
"""校验 Go 源码注释中文占比（仅注释，不含字符串）。"""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

from lang_verify_common import (
    cjk_ratio,
    collect_by_patterns,
    extract_go_comment_text,
    load_section,
)


def main() -> int:
    ap = argparse.ArgumentParser(description="校验 Go 注释中文占比")
    ap.add_argument("--file", type=Path, help="仅校验单个文件")
    args = ap.parse_args()

    com = load_section("commenting")
    min_ratio = float(com["min_cjk_ratio"])
    keywords = list(com.get("technical_keywords", []))
    paths = com["code_paths"]
    excludes = com["code_exclude"]

    def run_one(path: Path) -> tuple[bool, str]:
        rel = posix_rel(path)
        raw = path.read_text(encoding="utf-8")
        comments = extract_go_comment_text(raw)
        if not comments.strip():
            return True, ""
        r = cjk_ratio(comments, keywords)
        if r + 1e-9 < min_ratio:
            return False, f"{path}: 注释中文占比 {r:.3f} < 阈值 {min_ratio}"
        return True, ""

    if args.file:
        ok, msg = run_one(args.file.resolve())
        if not ok:
            print(msg, file=sys.stderr)
            return 1
        return 0

    bad: list[str] = []
    for path in collect_by_patterns(paths, excludes):
        ok, msg = run_one(path)
        if not ok:
            bad.append(msg)
    if bad:
        print("\n".join(bad), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
