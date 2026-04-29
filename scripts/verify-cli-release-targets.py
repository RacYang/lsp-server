#!/usr/bin/env python3
from __future__ import annotations

import argparse
import pathlib
import re
import sys

ROOT = pathlib.Path(__file__).resolve().parents[1]
DEFAULT_CONFIG = ROOT / ".build" / "config.yaml"
MAKEFILE = ROOT / "Makefile"
GORELEASER = ROOT / ".goreleaser.yaml"


def read_cli_targets(path: pathlib.Path) -> set[str]:
    lines = path.read_text().splitlines()
    targets: set[str] = set()
    in_release = False
    in_targets = False
    for line in lines:
        if line.startswith("release:"):
            in_release = True
            in_targets = False
            continue
        if in_release and line and not line.startswith(" "):
            break
        if in_release and line.strip() == "cli_targets:":
            in_targets = True
            continue
        if in_targets:
            stripped = line.strip()
            if not stripped.startswith("- "):
                if stripped:
                    break
                continue
            targets.add(stripped[2:].strip().strip('"'))
    if not targets:
        raise ValueError(f"{path}: release.cli_targets 为空或缺失")
    return targets


def makefile_targets() -> set[str]:
    text = MAKEFILE.read_text()
    return {target for target in read_cli_targets(DEFAULT_CONFIG) if target in text}


def goreleaser_targets() -> set[str]:
    text = GORELEASER.read_text()
    goos = set(re.findall(r"^\s+- (darwin|linux|windows)$", text, flags=re.MULTILINE))
    goarch = set(re.findall(r"^\s+- (amd64|arm64)$", text, flags=re.MULTILINE))
    ignored = {
        f"{m.group(1)}/{m.group(2)}"
        for m in re.finditer(r"goos:\s*(\w+)\s*\n\s+goarch:\s*(\w+)", text)
    }
    return {f"{os}/{arch}" for os in goos for arch in goarch} - ignored


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--file", default=str(DEFAULT_CONFIG), help="用于负例测试的 config 文件")
    args = parser.parse_args()
    config_targets = read_cli_targets(pathlib.Path(args.file))
    sources = {
        "Makefile": makefile_targets(),
        ".goreleaser.yaml": goreleaser_targets(),
    }
    ok = True
    for name, targets in sources.items():
        if config_targets != targets:
            print(f"{name}: CLI release targets mismatch: config={sorted(config_targets)} actual={sorted(targets)}", file=sys.stderr)
            ok = False
    return 0 if ok else 1


if __name__ == "__main__":
    raise SystemExit(main())
