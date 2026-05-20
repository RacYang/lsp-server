#!/usr/bin/env python3
from __future__ import annotations

import pathlib
import re
import shutil
import subprocess
import sys


ROOT = pathlib.Path(__file__).resolve().parents[1]
RULES_DIR = ROOT / ".claude" / "rules"
CODEX_RULES_DIR = ROOT / ".codex" / "skills" / "lsp-server-governance" / "references" / "rules"
CONFIG_FILE = ROOT / ".build" / "config.yaml"
RULE_SCHEMA_FILE = ROOT / ".build" / "schema" / "rules.schema.json"
CLAUDE_FILE = ROOT / "CLAUDE.md"
AGENTS_FILE = ROOT / "AGENTS.md"
COMMANDS_DIR = ROOT / ".claude" / "commands"
TEMPLATES_DIR = ROOT / ".claude" / "templates"
ALLOWED_FIELDS = ("description", "alwaysApply", "globs")
ENFORCER_CONFIGS = {".commitlintrc.json", ".go-arch-lint.yml", ".golangci.yml", "buf.yaml", "Makefile"}


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


def extract_body_ref(text: str, label: str) -> str | None:
    """从规则正文的参考节中提取引用路径。

    .claude/rules/*.md 格式：
    - **ADR**：`docs/adr/0000-example.md`
    - **Enforcer**：`scripts/verify-example.py`
    - **负例**：`.build/negatives/example.go.neg`
    """
    pattern = rf"\*\*{re.escape(label)}\*\*[：:]\s*`([^`]+)`"
    m = re.search(pattern, text)
    return m.group(1) if m else None


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


def validate_rule_dir(rule_dir: pathlib.Path) -> None:
    for rule_file in sorted(rule_dir.glob("*.md")):
        data, order = parse_frontmatter(rule_file)
        if not data.get("description"):
            raise ValueError(f"{rule_file}: missing description")
        if "alwaysApply" in data and "globs" in data:
            raise ValueError(f"{rule_file}: alwaysApply and globs are mutually exclusive")
        if "alwaysApply" in data and data["alwaysApply"] != "true":
            raise ValueError(f"{rule_file}: alwaysApply must be true")

        body = rule_file.read_text()
        adr_ref = extract_body_ref(body, "ADR")
        enforcer_ref = extract_body_ref(body, "Enforcer")
        neg_ref = extract_body_ref(body, "负例")

        if not adr_ref:
            raise ValueError(f"{rule_file}: missing ADR reference in body")
        if not enforcer_ref:
            raise ValueError(f"{rule_file}: missing Enforcer reference in body")
        if not neg_ref:
            raise ValueError(f"{rule_file}: missing 负例 reference in body")

        require_file(adr_ref, rule_file)
        validate_enforcer(enforcer_ref, rule_file)
        require_file(neg_ref, rule_file)
        if not neg_ref.endswith(".neg"):
            raise ValueError(f"{rule_file}: negative_test must end with .neg")


def validate_rules() -> None:
    validate_rule_dir(RULES_DIR)
    validate_rule_dir(CODEX_RULES_DIR)


def find_yq() -> str:
    yq = shutil.which("yq")
    if yq is None:
        gopath = subprocess.run(["go", "env", "GOPATH"], check=True, capture_output=True, text=True).stdout.strip()
        candidate = pathlib.Path(gopath) / "bin" / "yq"
        yq = str(candidate) if candidate.exists() else "yq"
    return yq


def yq_value(expr: str) -> str:
    return subprocess.run(
        [find_yq(), "-r", expr, str(CONFIG_FILE)],
        check=True,
        capture_output=True,
        text=True,
    ).stdout.strip()


def validate_entry_file(path: pathlib.Path) -> None:
    text = path.read_text()
    max_lines = int(yq_value(".governance.entry_md_max_lines"))
    line_count = len(text.splitlines())
    if line_count > max_lines:
        raise ValueError(f"{path}: line count {line_count} exceeds {max_lines}")
    anchors = yq_value(".governance.entry_md_required_anchors[]").splitlines()
    for anchor in anchors:
        if anchor not in text:
            raise ValueError(f"{path}: missing required anchor {anchor}")


def validate_entry_files() -> None:
    validate_entry_file(CLAUDE_FILE)
    validate_entry_file(AGENTS_FILE)


def validate_governance_counts() -> None:
    rules_count = len(list(RULES_DIR.glob("*.md")))
    commands_count = len(list(COMMANDS_DIR.glob("*.md")))
    templates_count = len(list(TEMPLATES_DIR.glob("**/manifest.yaml"))) if TEMPLATES_DIR.exists() else 0
    limits = {
        "rules": (rules_count, int(yq_value(".governance.rules_max_count"))),
        "commands": (commands_count, int(yq_value(".governance.commands_max_count"))),
        "templates": (templates_count, int(yq_value(".governance.templates_max_count"))),
    }
    for name, (actual, limit) in limits.items():
        if actual > limit:
            raise ValueError(f"{name} count {actual} exceeds {limit}")


def validate_config_exists() -> None:
    if not CONFIG_FILE.exists():
        raise ValueError(f"missing config file: {CONFIG_FILE}")
    if not RULE_SCHEMA_FILE.exists():
        raise ValueError(f"missing rule schema file: {RULE_SCHEMA_FILE}")


def main() -> int:
    try:
        validate_config_exists()
        validate_rules()
        validate_entry_files()
        validate_governance_counts()
    except (ValueError, subprocess.CalledProcessError) as exc:
        print(str(exc), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
