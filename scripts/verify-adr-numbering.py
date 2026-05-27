#!/usr/bin/env python3
"""校验 ADR 编号连续且 README 已同步更新。"""

from __future__ import annotations

import re
import sys
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

    # 校验编号从 0000 开始且无缺口
    numbers = [int(p.stem[:4]) for p in md_files]
    expected = list(range(len(numbers)))
    if numbers != expected:
        missing = sorted(set(expected) - set(numbers))
        extra = sorted(set(numbers) - set(expected))
        if missing:
            errors.append(f"ADR 编号缺口：{[f'{n:04d}' for n in missing]}")
        if extra:
            errors.append(f"ADR 编号超出连续范围：{[f'{n:04d}' for n in extra]}")

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
