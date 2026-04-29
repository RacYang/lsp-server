#!/usr/bin/env python3
"""校验已存在的日志上下文边界文件注入必要字段。"""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

from lang_verify_common import ROOT, load_section


REQUIRED_CALLS = ("WithTraceID", "WithUserID", "WithRoomID")


def check_file(path: Path) -> list[str]:
    src = path.read_text(encoding="utf-8")
    errs: list[str] = []
    for name in REQUIRED_CALLS:
        if f".{name}(" not in src and f"logx.{name}(" not in src:
            errs.append(f"{path}: 日志边界缺少 logx.{name} 调用")
    return errs


def main() -> int:
    ap = argparse.ArgumentParser(description="校验日志上下文边界")
    ap.add_argument("--file", type=Path, help="仅校验单个文件")
    ap.add_argument("--strict", action="store_true", help="glob 命中 0 文件时报错")
    args = ap.parse_args()

    if args.file:
        errs = check_file(args.file.resolve())
        if errs:
            print("\n".join(errs), file=sys.stderr)
            return 1
        return 0

    cfg = load_section("logging").get("context_boundaries") or {}
    globs = list(cfg.get("globs") or [])
    all_errs: list[str] = []
    for pat in globs:
        matches = [p for p in ROOT.glob(pat) if p.is_file()]
        if not matches and args.strict:
            all_errs.append(f"{pat}: 未匹配到日志边界文件")
            continue
        for path in matches:
            all_errs.extend(check_file(path))
    if all_errs:
        print("\n".join(all_errs), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
