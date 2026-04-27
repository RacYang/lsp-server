#!/usr/bin/env python3
"""校验本地 topic 分支命名；main 与受保护分支放行，detached HEAD 可跳过。"""

from __future__ import annotations

import argparse
import re
import subprocess
import sys
from pathlib import Path

from lang_verify_common import load_section


def main() -> int:
    ap = argparse.ArgumentParser(description="校验 topic 分支命名")
    ap.add_argument("--file", type=Path, help="负例模式：文件首行为分支名")
    args = ap.parse_args()

    git_cfg = load_section("git")
    branch_cfg = git_cfg["branch"]
    pattern = re.compile(branch_cfg["topic_pattern"])
    allow_detached = bool(branch_cfg.get("allow_detached_head", True))
    allow_branches = set(branch_cfg.get("allow_branches") or [])
    protected = set(git_cfg.get("protected_branches") or [])
    default_branch = git_cfg.get("default_branch", "main")
    allow_branches.add(default_branch)
    allow_branches |= protected

    if args.file:
        name = args.file.read_text(encoding="utf-8").strip().splitlines()[0].strip()
    else:
        cur = subprocess.run(
            ["git", "rev-parse", "--abbrev-ref", "HEAD"],
            capture_output=True,
            text=True,
            check=False,
        )
        if cur.returncode != 0:
            print("非 git 仓库或未安装 git，跳过分支名校验", file=sys.stderr)
            return 0
        name = cur.stdout.strip()
        if name == "HEAD":
            if allow_detached:
                return 0
            print("detached HEAD 下无法校验 topic 分支命名", file=sys.stderr)
            return 1

    if name in allow_branches:
        return 0
    if pattern.fullmatch(name):
        return 0
    print(
        f"分支名不符合约定: {name!r}，应为 type/描述（type 为 feat|fix|docs|…），"
        f"或在 SSOT git.branch.allow_branches / protected_branches 中放行",
        file=sys.stderr,
    )
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
