#!/usr/bin/env python3
"""禁止业务代码直接 import 底层日志包；仅门面实现目录豁免。"""

from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path

from lang_verify_common import collect_by_patterns, is_facade_impl, load_section, posix_rel


def extract_import_paths(go_src: str) -> list[str]:
    paths: list[str] = []
    for m in re.finditer(r'^\s*import\s+(?:\w+\s+)?"([^"]+)"', go_src, re.MULTILINE):
        paths.append(m.group(1))
    for m in re.finditer(r"import\s+\((.*?)\)", go_src, re.DOTALL):
        inner = m.group(1)
        for sm in re.finditer(r'"([^"]+)"', inner):
            paths.append(sm.group(1))
    return paths


def _is_forbidden(imp: str, forbidden: set[str]) -> bool:
    if imp in forbidden:
        return True
    for fb in forbidden:
        if imp.startswith(fb + "/"):
            return True
    return False


def main() -> int:
    ap = argparse.ArgumentParser(description="禁止直调 slog/zap")
    ap.add_argument("--file", type=Path, help="仅校验单个文件")
    args = ap.parse_args()

    logcfg = load_section("logging")
    forbidden = set(logcfg["forbidden_packages"])
    impl_globs = list(logcfg["facade_impl_paths"])
    paths = load_section("commenting")["code_paths"]
    excludes = list(load_section("commenting")["code_exclude"])

    def check_file(path: Path) -> list[str]:
        rel = posix_rel(path)
        if is_facade_impl(rel, impl_globs):
            return []
        src = path.read_text(encoding="utf-8")
        hits: set[str] = set()
        for imp in extract_import_paths(src):
            if _is_forbidden(imp, forbidden):
                hits.add(f"{path}: 禁止 import `{imp}`，请改用日志门面")
        return sorted(hits)

    if args.file:
        errs = check_file(args.file.resolve())
        if errs:
            print("\n".join(errs), file=sys.stderr)
            return 1
        return 0

    all_errs: list[str] = []
    for path in collect_by_patterns(paths, excludes):
        all_errs.extend(check_file(path))
    if all_errs:
        print("\n".join(all_errs), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
