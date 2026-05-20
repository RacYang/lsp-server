#!/usr/bin/env python3
from __future__ import annotations

import json
import argparse
import pathlib
import re
import shutil
import subprocess
import sys
from typing import Any


ROOT = pathlib.Path(__file__).resolve().parents[1]
COMMANDS_DIR = ROOT / ".claude" / "commands"
RULES_DIR = ROOT / ".claude" / "rules"
TEMPLATES_DIR = ROOT / ".claude" / "templates"
CODEX_GOVERNANCE_DIR = ROOT / ".codex" / "skills" / "lsp-server-governance"
CODEX_WORKFLOWS_DIR = CODEX_GOVERNANCE_DIR / "references" / "workflows"
CODEX_RULES_DIR = CODEX_GOVERNANCE_DIR / "references" / "rules"
CODEX_TEMPLATES_DIR = CODEX_GOVERNANCE_DIR / "assets" / "templates"
MANIFEST_SCHEMA = ROOT / ".build" / "schema" / "manifest.schema.json"
COMMAND_ANCHORS = ("## When to use", "## Inputs", "## Steps", "## Verify")


def load_yaml_json(path: pathlib.Path) -> Any:
    yq = shutil.which("yq")
    if yq is None:
        gopath = subprocess.run(["go", "env", "GOPATH"], check=True, capture_output=True, text=True).stdout.strip()
        candidate = pathlib.Path(gopath) / "bin" / "yq"
        yq = str(candidate) if candidate.exists() else "yq"
    result = subprocess.run(
        [yq, "-o=json", ".", str(path)],
        check=True,
        capture_output=True,
        text=True,
    )
    return json.loads(result.stdout)


def validate_manifest(manifest: pathlib.Path) -> list[str]:
    errors: list[str] = []
    data = load_yaml_json(manifest)
    required = {"id", "purpose", "applies_to_glob", "coverage_cell", "verify"}
    missing = required - set(data)
    if missing:
        errors.append(f"{manifest}: missing keys: {', '.join(sorted(missing))}")
    if "id" in data and not re.fullmatch(r"[a-z][a-z0-9-]*", str(data["id"])):
        errors.append(f"{manifest}: invalid id")
    if "coverage_cell" in data and not re.fullmatch(r"[a-z]+/[a-z0-9-]+", str(data["coverage_cell"])):
        errors.append(f"{manifest}: invalid coverage_cell")
    for item in data.get("verify", []):
        if item.get("kind") not in {"presence", "regex", "script"}:
            errors.append(f"{manifest}: unsupported verify kind")
    return errors


def validate_commands(command_dir: pathlib.Path) -> list[str]:
    errors: list[str] = []
    for cmd in sorted(command_dir.glob("*.md")):
        text = cmd.read_text()
        for anchor in COMMAND_ANCHORS:
            if anchor not in text:
                errors.append(f"{cmd}: missing anchor {anchor}")
        forbidden = (".build/negatives/", "negative_test:")
        if cmd.name != "add-constraint.md" and any(token in text for token in forbidden):
            errors.append(f"{cmd}: command must not repeat rule negative-test details")
    return errors


def validate_command_file(cmd: pathlib.Path) -> list[str]:
    text = cmd.read_text()
    return [f"{cmd}: missing anchor {anchor}" for anchor in COMMAND_ANCHORS if anchor not in text]


def validate_rules(rule_dir: pathlib.Path) -> list[str]:
    errors: list[str] = []
    for rule in sorted(rule_dir.glob("*.md")):
        text = rule.read_text()
        if not text.startswith("---\n"):
            errors.append(f"{rule}: missing frontmatter start")
            continue
        parts = text.split("\n---\n", 1)
        if len(parts) != 2:
            errors.append(f"{rule}: missing frontmatter end")
            continue
        frontmatter = parts[0]
        body = parts[1] if len(parts) > 1 else ""
        if "description:" not in frontmatter:
            errors.append(f"{rule}: missing description in frontmatter")
        for token in ("**ADR**", "**Enforcer**", "**负例**"):
            if token not in body:
                errors.append(f"{rule}: missing {token} reference")
    return errors


def validate_templates(templates_dir: pathlib.Path) -> list[str]:
    errors: list[str] = []
    if not templates_dir.exists():
        return errors
    if not MANIFEST_SCHEMA.exists():
        errors.append(f"missing manifest schema: {MANIFEST_SCHEMA}")
    manifests = sorted(templates_dir.glob("**/manifest.yaml"))
    for manifest in manifests:
        try:
            errors.extend(validate_manifest(manifest))
        except (subprocess.CalledProcessError, json.JSONDecodeError) as exc:
            errors.append(f"{manifest}: {exc}")
    for template_dir in [path for path in templates_dir.glob("*/*") if path.is_dir()]:
        if not (template_dir / "manifest.yaml").exists():
            errors.append(f"{template_dir}: missing manifest.yaml")
    return errors


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--command-file")
    parser.add_argument("--manifest-file")
    args = parser.parse_args()

    if args.command_file:
        errors = validate_command_file(pathlib.Path(args.command_file))
    elif args.manifest_file:
        errors = validate_manifest(pathlib.Path(args.manifest_file))
    else:
        errors = (
            validate_commands(COMMANDS_DIR)
            + validate_commands(CODEX_WORKFLOWS_DIR)
            + validate_rules(RULES_DIR)
            + validate_rules(CODEX_RULES_DIR)
            + validate_templates(TEMPLATES_DIR)
            + validate_templates(CODEX_TEMPLATES_DIR)
        )
    if errors:
        for error in errors:
            print(error, file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
