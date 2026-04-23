#!/usr/bin/env python3
from __future__ import annotations

import json
import pathlib
import re
import sys

from lang_verify_common import cjk_ratio


ROOT = pathlib.Path(__file__).resolve().parents[1]
CONFIG = json.loads((ROOT / ".commitlintrc.json").read_text())
HEADER = re.compile(CONFIG["headerPattern"])
TERMINAL_PUNCTUATION = re.compile(r"[。！？!?.,，；;：:]$")


def main() -> int:
    if len(sys.argv) != 2:
        print("用法: verify-commit-msg.py <path>", file=sys.stderr)
        return 1
    msg = pathlib.Path(sys.argv[1]).read_text().strip().splitlines()[0]
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
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
