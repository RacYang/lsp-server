#!/usr/bin/env python3
"""校验观测规则引用的存储操作标签能在代码中找到。"""

from __future__ import annotations

import re
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
OBSERVE_STORAGE = re.compile(r'ObserveStorage\("(?P<store>[^"]+)",\s*"(?P<op>[^"]+)"')
RULE_STORAGE = re.compile(r'lsp_storage_op_seconds_bucket\{(?P<labels>[^}]*)\}')
LABEL = re.compile(r'(?P<key>[a-zA-Z_][a-zA-Z0-9_]*)="(?P<value>[^"]+)"')


def collect_code_ops() -> set[tuple[str, str]]:
    out: set[tuple[str, str]] = set()
    for path in sorted((ROOT / "internal").glob("**/*.go")):
        text = path.read_text(encoding="utf-8")
        for match in OBSERVE_STORAGE.finditer(text):
            out.add((match.group("store"), match.group("op")))
    return out


def collect_rule_ops() -> list[tuple[Path, str, str]]:
    out: list[tuple[Path, str, str]] = []
    for path in sorted((ROOT / "deploy" / "observability").glob("*.yaml")):
        text = path.read_text(encoding="utf-8")
        for match in RULE_STORAGE.finditer(text):
            labels = {m.group("key"): m.group("value") for m in LABEL.finditer(match.group("labels"))}
            store = labels.get("store")
            op = labels.get("op")
            if store and op:
                out.append((path, store, op))
    return out


def main() -> int:
    code_ops = collect_code_ops()
    errors: list[str] = []
    for path, store, op in collect_rule_ops():
        if (store, op) not in code_ops:
            rel = path.relative_to(ROOT).as_posix()
            errors.append(f"{rel}: 观测规则引用了不存在的存储操作标签 store={store!r}, op={op!r}")
    if errors:
        print("\n".join(errors), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
