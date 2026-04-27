#!/usr/bin/env python3
"""校验 Markdown / MDC / SKILL 正文中文占比（配置驱动）。"""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

from lang_verify_common import (
    cjk_ratio,
    collect_by_patterns,
    load_section,
    strip_markdown_body,
)


def check_file(path: Path, min_ratio: float) -> tuple[bool, str]:
    raw = path.read_text(encoding="utf-8")
    body = strip_markdown_body(raw)
    r = cjk_ratio(body, None)
    if r + 1e-9 < min_ratio:
        return False, f"{path}: 中文占比 {r:.3f} < 阈值 {min_ratio}"
    return True, ""


def main() -> int:
    ap = argparse.ArgumentParser(description="校验文档中文占比")
    ap.add_argument("--file", type=Path, help="仅校验单个文件（负例模式）")
    args = ap.parse_args()

    lang = load_section("language")
    min_ratio = float(lang["docs_min_cjk_ratio"])

    if args.file:
        p = args.file.resolve()
        ok, msg = check_file(p, min_ratio)
        if not ok:
            print(msg, file=sys.stderr)
            return 1
        return 0

    paths = lang["docs_paths"]
    excludes = lang["docs_exclude"]
    bad: list[str] = []
    for path in collect_by_patterns(paths, excludes):
        ok, msg = check_file(path, min_ratio)
        if not ok:
            bad.append(msg)
    if bad:
        print("\n".join(bad), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
