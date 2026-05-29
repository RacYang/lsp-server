#!/usr/bin/env python3
"""校验 ADR-0045 阶段所有权：phaseReason / phaseStartUnixMs 仅允许在 phase.go 的
enterPhase 与持久化恢复路径（engine_persist.go 的 buildRoundStateFromPersist /
finalizeRoundInvariants）中直接赋值。

任何其它路径若直接写 rs.phaseReason / rs.phaseStartUnixMs，必须改为调用
rs.enterPhase(reason)，由其原子设置 phaseReason / phaseStartUnixMs / deadlineUnixMs
与等待态布尔标志，从结构上消除 stale-deadline 错位。
"""

from __future__ import annotations

import re
import sys
from pathlib import Path

ROOT = Path.cwd()
TARGET_DIR = ROOT / "internal" / "service" / "room"
ALLOWED_FILES = {"phase.go", "engine_persist.go"}
# 使用 \. 而非 \brs\. 以覆盖所有变量名（包括 initialRound、rs 等），
# 避免因变量命名差异导致漏检。同时补充 deadlineUnixMs 以完整覆盖 ADR-0045 保护字段。
PATTERNS = [
    re.compile(r"\.phaseReason\s*="),
    re.compile(r"\.phaseStartUnixMs\s*="),
    re.compile(r"\.deadlineUnixMs\s*="),
]


def main() -> int:
    failures: list[str] = []
    if not TARGET_DIR.is_dir():
        return 0
    for path in sorted(TARGET_DIR.glob("*.go")):
        if path.name in ALLOWED_FILES:
            continue
        if path.name.endswith("_test.go"):
            continue
        text = path.read_text(encoding="utf-8")
        for lineno, line in enumerate(text.splitlines(), start=1):
            for pat in PATTERNS:
                if pat.search(line):
                    rel = path.relative_to(ROOT).as_posix()
                    failures.append(f"{rel}:{lineno}: 禁止直接赋值 phaseReason/phaseStartUnixMs，应调用 rs.enterPhase()；详见 ADR-0045")
    if failures:
        sys.stderr.write("\n".join(failures) + "\n")
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
