#!/usr/bin/env python3
"""校验本次推送涉及的 tag 名称；release 须匹配 vX.Y.Z，特殊名在白名单。"""

from __future__ import annotations

import argparse
import re
import subprocess
import sys
from pathlib import Path

from lang_verify_common import load_section


def _parse_push_lines(text: str) -> list[str]:
    tags: list[str] = []
    for line in text.splitlines():
        line = line.strip()
        if not line:
            continue
        parts = line.split()
        if len(parts) < 4:
            continue
        local_ref = parts[0]
        if local_ref.startswith("refs/tags/"):
            tags.append(local_ref[len("refs/tags/") :])
    return tags


def main() -> int:
    ap = argparse.ArgumentParser(description="校验推送中的 git tag 命名")
    ap.add_argument("--file", type=Path, help="负例模式：每行一个 tag 名")
    args = ap.parse_args()

    git_cfg = load_section("git")
    tags_cfg = git_cfg["tags"]
    release_re = re.compile(tags_cfg["release_pattern"])
    allow_special = set(tags_cfg.get("allow_special") or [])

    if args.file:
        tag_names = [
            ln.strip()
            for ln in args.file.read_text(encoding="utf-8").splitlines()
            if ln.strip()
        ]
    else:
        stdin = sys.stdin.read()
        if not stdin.strip():
            return 0
        tag_names = _parse_push_lines(stdin)

    bad: list[str] = []
    for name in tag_names:
        if name in allow_special:
            continue
        if release_re.fullmatch(name):
            continue
        bad.append(name)
    if bad:
        print(
            "以下 tag 不符合发布命名或不在白名单: "
            + ", ".join(repr(b) for b in bad),
            file=sys.stderr,
        )
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
