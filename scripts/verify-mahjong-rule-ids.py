#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import pathlib
import re
import shutil
import subprocess
import sys


ROOT = pathlib.Path(__file__).resolve().parents[1]
CONFIG_FILE = ROOT / ".build" / "config.yaml"
EXCLUDED_DIRS = {".git", ".cursor", "api/gen", ".build/negatives"}
FORBIDDEN_LEGACY_TOKENS = ("sichuan_" + "xzdd", "sichuan" + "xzdd")


def load_rule_config() -> dict[str, object]:
    yq = shutil.which("yq")
    if yq is None:
        gopath = subprocess.check_output(["go", "env", "GOPATH"], text=True).strip()
        candidate = pathlib.Path(gopath) / "bin" / "yq"
        if candidate.exists():
            yq = str(candidate)
    if yq is None:
        raise ValueError("missing yq")
    raw = subprocess.check_output([yq, "-o=json", ".mahjong.rules", str(CONFIG_FILE)], text=True)
    cfg = json.loads(raw)
    if not isinstance(cfg, dict):
        raise ValueError("missing mahjong.rules config")
    return cfg


def validate_rule_id(rule_id: str, pattern: re.Pattern[str], forbidden: set[str]) -> list[str]:
    errors: list[str] = []
    if not pattern.fullmatch(rule_id):
        errors.append(f"规则 ID 不符合格式: {rule_id}")
    parts = rule_id.split("_")
    for part in parts:
        if part in forbidden:
            errors.append(f"规则 ID 使用了禁用缩写 {part}: {rule_id}")
    return errors


def iter_repo_text_files() -> list[pathlib.Path]:
    files: list[pathlib.Path] = []
    for path in ROOT.rglob("*"):
        rel = path.relative_to(ROOT).as_posix()
        if rel == "scripts/verify-mahjong-rule-ids.py":
            continue
        if any(rel == item or rel.startswith(item + "/") for item in EXCLUDED_DIRS):
            continue
        if path.is_dir() or path.suffix in {".sum", ".pb.go"}:
            continue
        files.append(path)
    return files


def text_contains_legacy_token(path: pathlib.Path) -> str | None:
    try:
        text = path.read_text()
    except UnicodeDecodeError:
        return None
    for token in FORBIDDEN_LEGACY_TOKENS:
        if token in text:
            return token
    return None


def validate_candidate_file(path: pathlib.Path, pattern: re.Pattern[str], forbidden: set[str]) -> list[str]:
    text = path.read_text()
    candidates = re.findall(r"[a-z][a-z0-9_]*", text)
    errors: list[str] = []
    for candidate in candidates:
        if candidate in FORBIDDEN_LEGACY_TOKENS or any(part in forbidden for part in candidate.split("_")):
            errors.append(f"{path}: 使用了禁用缩写或旧规则 ID: {candidate}")
        elif candidate.count("_") >= 2:
            errors.extend(f"{path}: {err}" for err in validate_rule_id(candidate, pattern, forbidden))
    return errors


def main() -> int:
    parser = argparse.ArgumentParser(description="校验麻将规则 ID 命名与白名单")
    parser.add_argument("--file", type=pathlib.Path, help="只校验单个负例文件")
    args = parser.parse_args()

    try:
        cfg = load_rule_config()
        pattern = re.compile(str(cfg["id_pattern"]))
        forbidden = {str(item) for item in cfg["forbidden_abbreviations"]}
        allowed_ids = [str(item) for item in cfg["allowed_ids"]]
    except (KeyError, ValueError, subprocess.CalledProcessError, re.error) as exc:
        print(f"读取麻将规则 ID 配置失败: {exc}", file=sys.stderr)
        return 1

    errors: list[str] = []
    for rule_id in allowed_ids:
        errors.extend(validate_rule_id(rule_id, pattern, forbidden))

    if args.file is not None:
        errors.extend(validate_candidate_file(args.file, pattern, forbidden))
    else:
        source_text = "\n".join(
            path.read_text(errors="ignore")
            for path in (ROOT / "internal" / "mahjong").rglob("*.go")
            if ".build/negatives" not in path.as_posix()
        )
        for rule_id in allowed_ids:
            if rule_id not in source_text:
                errors.append(f"allowed_ids 中的规则未在麻将实现中出现: {rule_id}")
        for path in iter_repo_text_files():
            token = text_contains_legacy_token(path)
            if token:
                errors.append(f"{path.relative_to(ROOT)}: 禁止继续使用旧规则 ID 或包名片段 {token}")

    if errors:
        for err in errors:
            print(err, file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
