#!/usr/bin/env python3
"""校验 pre-commit、pre-push 与 CI 是否调用 SSOT 指定的 make 目标。"""

from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path

from lang_verify_common import load_section


ROOT = Path(__file__).resolve().parents[1]


def _read(path: Path) -> str:
    return path.read_text(encoding="utf-8")


def _has_make_target(text: str, target: str) -> bool:
    """匹配 workflow 中的 `run: make <target>` 或 shell 脚本行 `make <target>`。"""
    esc = re.escape(target)
    yaml_pat = rf"run:\s*make\s+{esc}\b"
    sh_pat = rf"^\s*make\s+{esc}\b"
    return bool(
        re.search(yaml_pat, text, re.MULTILINE)
        or re.search(sh_pat, text, re.MULTILINE)
    )


def main() -> int:
    ap = argparse.ArgumentParser(description="校验 hook 与 CI 的 make 目标映射")
    ap.add_argument("--file", type=Path, help="负例模式：单个 workflow YAML 片段文件")
    args = ap.parse_args()

    git_cfg = load_section("git")
    parity = git_cfg["ci_parity"]
    pre_commit_target = parity["pre_commit_target"]
    pre_push_target = parity["pre_push_target"]
    ci_target = parity["ci_target"]

    if args.file:
        ci_text = _read(args.file.resolve())
        if not _has_make_target(ci_text, ci_target):
            print(
                f"片段缺少 make {ci_target}，不符合 hooks parity 约定",
                file=sys.stderr,
            )
            return 1
        return 0

    pre_commit = _read(ROOT / ".githooks" / "pre-commit")
    pre_push = _read(ROOT / ".githooks" / "pre-push")
    ci_text = _read(ROOT / ".github" / "workflows" / "ci.yml")

    errs: list[str] = []
    if not _has_make_target(pre_commit, pre_commit_target):
        errs.append(f".githooks/pre-commit 应调用 make {pre_commit_target}")
    if not _has_make_target(pre_push, "verify-git-push"):
        errs.append(".githooks/pre-push 应调用 make verify-git-push")
    if not _has_make_target(pre_push, pre_push_target):
        errs.append(f".githooks/pre-push 应调用 make {pre_push_target}")
    if not _has_make_target(ci_text, ci_target):
        errs.append(f".github/workflows/ci.yml 应调用 make {ci_target}")

    if errs:
        print("\n".join(errs), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
