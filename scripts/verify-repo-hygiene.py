#!/usr/bin/env python3
"""校验被跟踪文件体积、禁用文件名与二进制策略（可配置白名单）。"""

from __future__ import annotations

import argparse
import codecs
import fnmatch
import subprocess
import sys
from pathlib import Path

from lang_verify_common import excluded, load_section, posix_rel


def _is_likely_binary(path: Path) -> bool:
    """含 NUL 或无法用 UTF-8 解码的样本视为二进制；中文等 UTF-8 文本不误伤。"""
    try:
        data = path.read_bytes()[:8000]
    except OSError:
        return False
    if b"\x00" in data:
        return True
    try:
        codecs.getincrementaldecoder("utf-8")().decode(data, final=False)
    except UnicodeDecodeError:
        return True
    return False


def main() -> int:
    ap = argparse.ArgumentParser(description="校验仓库被跟踪文件卫生")
    ap.add_argument("--file", type=Path, help="负例模式：单行相对路径")
    args = ap.parse_args()

    git_cfg = load_section("git")
    hy = git_cfg["repo_hygiene"]
    max_bytes = int(hy["max_tracked_file_bytes"])
    forbidden = {str(x) for x in hy.get("forbidden_basenames") or []}
    binary_blocked = bool(hy.get("binary_blocked", True))
    allow_bin = list(hy.get("allow_binary_globs") or [])
    allow_large = list(hy.get("allow_large_file_globs") or [])

    root = Path(__file__).resolve().parents[1]

    if args.file:
        rel = args.file.read_text(encoding="utf-8").strip().splitlines()[0].strip()
        base = Path(rel).name
        if base in forbidden:
            print(f"{rel}: 禁止跟踪该文件名", file=sys.stderr)
            return 1
        path = root / rel
        if not path.is_file():
            print(f"{rel}: 路径不存在", file=sys.stderr)
            return 1
        paths = [path]
    else:
        out = subprocess.run(
            ["git", "ls-files", "-z"],
            cwd=root,
            capture_output=True,
            check=False,
        )
        if out.returncode != 0:
            print("git ls-files 失败，跳过仓库卫生校验", file=sys.stderr)
            return 0
        raw = out.stdout.split(b"\0")
        paths = [root / p.decode("utf-8", errors="replace") for p in raw if p]

    errs: list[str] = []
    for path in paths:
        if not path.is_file():
            continue
        rel = posix_rel(path)
        if excluded(rel, allow_bin) or excluded(rel, allow_large):
            continue
        base = path.name
        if base in forbidden:
            errs.append(f"{rel}: 禁止跟踪该文件名")
            continue
        st = path.stat()
        if st.st_size > max_bytes and not excluded(rel, allow_large):
            errs.append(f"{rel}: 文件过大 {st.st_size} > {max_bytes}")
            continue
        if binary_blocked and not excluded(rel, allow_bin):
            if _is_likely_binary(path):
                errs.append(f"{rel}: 禁止跟踪二进制文件（或加入 allow_binary_globs）")

    if errs:
        print("\n".join(errs), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
