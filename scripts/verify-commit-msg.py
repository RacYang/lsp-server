#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import pathlib
import re
import sys

from lang_verify_common import cjk_ratio


ROOT = pathlib.Path(__file__).resolve().parents[1]
CONFIG = json.loads((ROOT / ".commitlintrc.json").read_text())
HEADER = re.compile(CONFIG["headerPattern"])
TERMINAL_PUNCTUATION = re.compile(r"[。！？!?.,，；;：:]$")
TRAILER = re.compile(r"^([A-Za-z0-9-]+):[ \t]*(.*)$")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(add_help=False)
    parser.add_argument("--strip-forbidden-trailers", action="store_true")
    parser.add_argument("path")
    return parser.parse_args()


def normalize_forbidden_trailers() -> set[str]:
    return {str(x).casefold() for x in CONFIG.get("forbiddenTrailers", [])}


def strip_forbidden_trailers(path: pathlib.Path, forbidden: set[str]) -> None:
    lines = path.read_text().splitlines()
    filtered: list[str] = []
    for line in lines:
        match = TRAILER.match(line)
        if match and match.group(1).casefold() in forbidden:
            continue
        filtered.append(line)
    while filtered and filtered[-1] == "":
        filtered.pop()
    path.write_text("\n".join(filtered) + ("\n" if filtered else ""))


def main() -> int:
    args = parse_args()
    path = pathlib.Path(args.path)
    forbidden_trailers = normalize_forbidden_trailers()
    if args.strip_forbidden_trailers and forbidden_trailers:
        strip_forbidden_trailers(path, forbidden_trailers)
    lines = path.read_text().strip().splitlines()
    if not lines:
        print("提交信息不能为空", file=sys.stderr)
        return 1
    msg = lines[0]
    match = HEADER.match(msg)
    if not match:
        print("提交标题格式无效，应为 type(scope): 摘要 或 type: 摘要", file=sys.stderr)
        return 1
    msg_type = match.group(1)
    scope = match.group(3)
    summary = match.group(4).strip()
    if msg_type not in CONFIG["types"]:
        print(f"不支持的提交类型: {msg_type}", file=sys.stderr)
        return 1
    if CONFIG.get("scopesRequired") and not scope:
        print("提交 scope 不能为空", file=sys.stderr)
        return 1
    scope_pattern = CONFIG.get("scopePattern")
    if scope and scope_pattern and not re.fullmatch(scope_pattern, scope):
        print("提交 scope 必须使用小写英文、数字、短横线或 /", file=sys.stderr)
        return 1
    min_ratio = float(CONFIG.get("summaryMinCjkRatio", 0))
    ratio = cjk_ratio(summary)
    if ratio + 1e-9 < min_ratio:
        print(f"提交摘要中文占比 {ratio:.3f} < 阈值 {min_ratio}", file=sys.stderr)
        return 1
    if CONFIG.get("summaryDisallowTerminalPunctuation") and TERMINAL_PUNCTUATION.search(summary):
        print("提交摘要末尾不能带句号、感叹号等收尾标点", file=sys.stderr)
        return 1
    for line in lines[1:]:
        trailer = TRAILER.match(line)
        if not trailer:
            continue
        if trailer.group(1).casefold() in forbidden_trailers:
            print(f"禁止的提交 Trailer: {trailer.group(1)}", file=sys.stderr)
            return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
