#!/usr/bin/env python3
"""校验同集群 gate 恢复不得依赖历史 advertise 字段做控制流。"""

from __future__ import annotations

import argparse
import pathlib
import sys


ROOT = pathlib.Path(__file__).resolve().parents[1]

FORBIDDEN = (
    "currentGateAdvertiseAddr",
    "会话绑定在其他网关节点",
    "srec.AdvertiseAddr",
    "deps.Session.Issue(ctx, state.userID, r.Host)",
)


def iter_go_files(root: pathlib.Path) -> list[pathlib.Path]:
    targets = [root / "internal" / "app", root / "internal" / "handler", root / "internal" / "session"]
    files: list[pathlib.Path] = []
    for target in targets:
        if target.exists():
            files.extend(p for p in target.rglob("*.go") if not p.name.endswith("_test.go"))
    return files


def check_file(path: pathlib.Path) -> list[str]:
    text = path.read_text(encoding="utf-8")
    rel = path.relative_to(ROOT) if path.is_relative_to(ROOT) else path
    return [f"{rel}: 禁止在 gate 会话恢复控制流中使用 {pattern!r}" for pattern in FORBIDDEN if pattern in text]


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--file", type=pathlib.Path, help="仅校验单个负例文件")
    args = parser.parse_args()

    files = [args.file] if args.file else iter_go_files(ROOT)
    errors: list[str] = []
    for path in files:
        errors.extend(check_file(path))
    if errors:
        print("\n".join(errors), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
