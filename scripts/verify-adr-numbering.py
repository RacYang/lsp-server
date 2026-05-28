#!/usr/bin/env python3
"""校验 ADR 编号连续且 README 已同步更新。"""

from __future__ import annotations

import sys
from collections import Counter
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
ADR_DIR = ROOT / "docs" / "adr"


def main() -> int:
    md_files = sorted(
        p for p in ADR_DIR.glob("[0-9][0-9][0-9][0-9]-*.md")
    )
    if not md_files:
        print("verify-adr-numbering: docs/adr/ 中无 ADR 文件", file=sys.stderr)
        return 1

    errors: list[str] = []

    numbers = [int(p.stem[:4]) for p in md_files]

    # 先检测重复编号（set 操作会掩盖重复，必须在 set 之前检查）
    dupes = sorted(n for n, c in Counter(numbers).items() if c > 1)
    if dupes:
        errors.append(f"ADR 编号重复：{[f'{n:04d}' for n in dupes]}")

    # 校验编号从 0000 开始且无缺口（expected 覆盖 [0, max] 完整区间）
    expected = list(range(numbers[-1] + 1))
    nums_set = set(numbers)
    missing = sorted(set(expected) - nums_set)
    if missing:
        errors.append(f"ADR 编号缺口：{[f'{n:04d}' for n in missing]}")

    # 校验 README 引用所有 ADR 文件
    readme = ADR_DIR / "README.md"
    if not readme.exists():
        errors.append("docs/adr/README.md 不存在")
    else:
        readme_text = readme.read_text(encoding="utf-8")
        for p in md_files:
            if p.name not in readme_text:
                errors.append(f"docs/adr/README.md 未引用 {p.name}")

    if errors:
        for e in errors:
            print(f"verify-adr-numbering: {e}", file=sys.stderr)
        return 1

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
