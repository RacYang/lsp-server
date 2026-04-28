#!/usr/bin/env python3
from __future__ import annotations

import pathlib
import sys
from collections.abc import Iterable


ROOT = pathlib.Path(__file__).resolve().parents[1]
RULES_DIR = ROOT / ".cursor" / "rules"
CONFIG_FILE = ROOT / ".build" / "config.yaml"
RULE_SCHEMA_FILE = ROOT / ".build" / "schema" / "rules.schema.json"
ALLOWED_FIELDS = ("kind", "description", "alwaysApply", "globs", "adr", "enforcer", "negative_test")
ENFORCER_CONFIGS = {".commitlintrc.json", ".go-arch-lint.yml", ".golangci.yml", "buf.yaml"}


def parse_frontmatter(path: pathlib.Path) -> tuple[dict[str, str], list[str]]:
    text = path.read_text()
    if not text.startswith("---\n"):
        raise ValueError(f"{path}: missing frontmatter start")
    parts = text.split("\n---\n", 1)
    if len(parts) != 2:
        raise ValueError(f"{path}: missing frontmatter end")
    frontmatter: dict[str, str] = {}
    order: list[str] = []
    for raw_line in parts[0].splitlines()[1:]:
        line = raw_line.strip()
        if not line or line.startswith("#"):
            continue
        key, _, value = line.partition(":")
        if not _:
            raise ValueError(f"{path}: invalid frontmatter line: {raw_line}")
        key = key.strip()
        frontmatter[key] = value.strip().strip('"')
        order.append(key)
    return frontmatter, order


def require_file(path_str: str, owner: pathlib.Path) -> None:
    target = ROOT / path_str
    if not target.exists():
        raise ValueError(f"{owner}: referenced file does not exist: {path_str}")


def require_order(order: Iterable[str], owner: pathlib.Path) -> None:
    seen_positions = []
    for key in order:
        if key not in ALLOWED_FIELDS:
            raise ValueError(f"{owner}: unsupported frontmatter field: {key}")
        seen_positions.append(ALLOWED_FIELDS.index(key))
    if seen_positions != sorted(seen_positions):
        raise ValueError(f"{owner}: frontmatter fields must follow order: {', '.join(ALLOWED_FIELDS)}")


def validate_enforcer(enforcer: str, owner: pathlib.Path) -> None:
    base = enforcer.split("#", 1)[0]
    if base.startswith("scripts/"):
        if not (base.endswith(".py") or base.endswith(".sh")):
            raise ValueError(f"{owner}: script enforcer must end with .py or .sh: {enforcer}")
        require_file(base, owner)
        return
    if base in ENFORCER_CONFIGS:
        require_file(base, owner)
        return
    raise ValueError(f"{owner}: unsupported enforcer: {enforcer}")


def validate_rules() -> None:
    for rule_file in sorted(RULES_DIR.glob("*.mdc")):
        data, order = parse_frontmatter(rule_file)
        require_order(order, rule_file)
        kind = data.get("kind")
        if kind not in {"constraint", "norm"}:
            raise ValueError(f"{rule_file}: invalid kind")
        if not data.get("description"):
            raise ValueError(f"{rule_file}: missing description")
        if "alwaysApply" in data and "globs" in data:
            raise ValueError(f"{rule_file}: alwaysApply and globs are mutually exclusive")
        if "alwaysApply" in data and data["alwaysApply"] != "true":
            raise ValueError(f"{rule_file}: alwaysApply must be true")
        if kind == "constraint":
            for key in ("adr", "enforcer", "negative_test"):
                if not data.get(key):
                    raise ValueError(f"{rule_file}: missing {key}")
            require_file(data["adr"], rule_file)
            validate_enforcer(data["enforcer"], rule_file)
            require_file(data["negative_test"], rule_file)
            if not data["negative_test"].endswith(".neg"):
                raise ValueError(f"{rule_file}: negative_test must end with .neg")
        else:
            if not data.get("adr"):
                raise ValueError(f"{rule_file}: norm rules require adr")
            require_file(data["adr"], rule_file)
            for key in ("globs", "enforcer", "negative_test"):
                if key in data:
                    raise ValueError(f"{rule_file}: norm rules must not define {key}")


def validate_config_exists() -> None:
    if not CONFIG_FILE.exists():
        raise ValueError(f"missing config file: {CONFIG_FILE}")
    if not RULE_SCHEMA_FILE.exists():
        raise ValueError(f"missing rule schema file: {RULE_SCHEMA_FILE}")


def main() -> int:
    try:
        validate_config_exists()
        validate_rules()
    except ValueError as exc:
        print(str(exc), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
