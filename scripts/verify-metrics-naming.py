#!/usr/bin/env python3
"""校验 Prometheus 指标命名与后缀约定。"""

from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path

from lang_verify_common import ROOT, load_section, posix_rel


METRIC_BLOCK = re.compile(
    r"prom(?:auto)?\.New(?:Counter|CounterVec|Gauge|GaugeVec|Histogram|HistogramVec|Summary|SummaryVec)"
    r"\(prometheus\.[A-Za-z]+Opts\{(?P<body>.*?)\}",
    re.DOTALL,
)
FIELD = re.compile(r"(?P<name>Namespace|Name):\s*\"(?P<value>[^\"]+)\"")


def check_source(path: Path, name_prefix: str, allowed_suffixes: tuple[str, ...]) -> list[str]:
    source = path.read_text(encoding="utf-8")
    rel = posix_rel(path.resolve())
    errors: list[str] = []
    for block in METRIC_BLOCK.finditer(source):
        fields = {m.group("name"): m.group("value") for m in FIELD.finditer(block.group("body"))}
        namespace = fields.get("Namespace", "")
        name = fields.get("Name")
        if not name:
            errors.append(f"{rel}: Prometheus 指标必须显式填写 Name")
            continue
        full_name = f"{namespace}_{name}" if namespace else name
        if not full_name.startswith(name_prefix):
            errors.append(f"{rel}: 指标名必须以 {name_prefix!r} 开头: {full_name!r}")
        if not full_name.endswith(allowed_suffixes):
            suffixes = ", ".join(allowed_suffixes)
            errors.append(f"{rel}: 指标名后缀不在允许集合 [{suffixes}]: {full_name!r}")
    return errors


def target_files(single_file: Path | None) -> list[Path]:
    if single_file:
        return [single_file.resolve()]
    return sorted(p for p in (ROOT / "internal" / "metrics").glob("**/*.go") if p.is_file())


def main() -> int:
    parser = argparse.ArgumentParser(description="校验 Prometheus 指标命名")
    parser.add_argument("--file", type=Path, help="仅校验单个文件（负例模式）")
    args = parser.parse_args()

    config = load_section("metrics")
    name_prefix = str(config["name_prefix"])
    allowed_suffixes = tuple(str(s) for s in config["allowed_suffixes"])
    errors: list[str] = []
    for path in target_files(args.file):
        errors.extend(check_source(path, name_prefix, allowed_suffixes))
    if errors:
        print("\n".join(errors), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
