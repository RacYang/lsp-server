#!/usr/bin/env python3
from __future__ import annotations

import pathlib
import sys


ROOT = pathlib.Path(__file__).resolve().parents[1]
RULES_DIR = ROOT / ".cursor" / "rules"
CONFIG_FILE = ROOT / ".build" / "config.yaml"


def parse_frontmatter(path: pathlib.Path) -> dict[str, str]:
    text = path.read_text()
    if not text.startswith("---\n"):
        raise ValueError(f"{path}: missing frontmatter start")
    parts = text.split("\n---\n", 1)
    if len(parts) != 2:
        raise ValueError(f"{path}: missing frontmatter end")
    frontmatter = {}
    for raw_line in parts[0].splitlines()[1:]:
        line = raw_line.strip()
        if not line or line.startswith("#"):
            continue
        key, _, value = line.partition(":")
        if not _:
            raise ValueError(f"{path}: invalid frontmatter line: {raw_line}")
        frontmatter[key.strip()] = value.strip()
    return frontmatter


def require_file(path_str: str, owner: pathlib.Path) -> None:
    target = ROOT / path_str
    if not target.exists():
        raise ValueError(f"{owner}: referenced file does not exist: {path_str}")


def validate_rules() -> None:
    for rule_file in sorted(RULES_DIR.glob("*.mdc")):
        data = parse_frontmatter(rule_file)
        kind = data.get("kind")
        if kind not in {"constraint", "norm"}:
            raise ValueError(f"{rule_file}: invalid kind")
        if not data.get("description"):
            raise ValueError(f"{rule_file}: missing description")
        if kind == "constraint":
            for key in ("adr", "enforcer", "negative_test"):
                if not data.get(key):
                    raise ValueError(f"{rule_file}: missing {key}")
            require_file(data["adr"], rule_file)
            require_file(data["negative_test"], rule_file)
            if not data["negative_test"].endswith(".neg"):
                raise ValueError(f"{rule_file}: negative_test must end with .neg")
        else:
            if not data.get("adr"):
                raise ValueError(f"{rule_file}: norm rules require adr")
            require_file(data["adr"], rule_file)


def validate_config_exists() -> None:
    if not CONFIG_FILE.exists():
        raise ValueError(f"missing config file: {CONFIG_FILE}")


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
