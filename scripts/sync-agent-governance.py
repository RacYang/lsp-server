#!/usr/bin/env python3
"""同步 Claude 与 Codex 两套 Agent 治理资产。"""

from __future__ import annotations

import argparse
import filecmp
import pathlib
import shutil
import sys


ROOT = pathlib.Path(__file__).resolve().parents[1]
CLAUDE_DIR = ROOT / ".claude"
CODEX_SKILL_DIR = ROOT / ".codex" / "skills" / "lsp-server-governance"
CODEX_RULES_DIR = CODEX_SKILL_DIR / "references" / "rules"
CODEX_WORKFLOWS_DIR = CODEX_SKILL_DIR / "references" / "workflows"
CODEX_TEMPLATES_DIR = CODEX_SKILL_DIR / "assets" / "templates"
SKILL_FILE = CODEX_SKILL_DIR / "SKILL.md"


SKILL_BODY = """---
name: lsp-server-governance
description: Use when working in the lsp-server repository, including bug fixes, Go packages, proto contracts, mahjong rules, tests, observability, governance assets, templates, commits, pushes, and release workflows. This skill routes Codex to the mirrored Claude rules, workflows, templates, and project documentation.
---

# lsp-server 治理技能

## 使用时机

在本仓库处理代码、协议、麻将规则、房间服务、CLI、观测、文档、治理资产、提交或推送时使用。先读 `AGENTS.md`，再按任务读取对应 workflow、rule、template 与 ADR。

## 入口

- 项目入口：`AGENTS.md`
- 覆盖矩阵：`docs/agent-governance/coverage-matrix.md`
- 规则索引：`.codex/skills/lsp-server-governance/references/rules/`
- 工作流索引：`.codex/skills/lsp-server-governance/references/workflows/`
- 模板资产：`.codex/skills/lsp-server-governance/assets/templates/`

## 工作方式

1. 根据 `AGENTS.md` 的任务路由选择 workflow。
2. 读取 workflow 的 `When to use`、`Inputs`、`Steps`、`Verify`。
3. 按影响面读取相关 rule，遵守 ADR、Enforcer、负例三元组。
4. 需要骨架时优先使用 templates 中的 manifest 与模板文件。
5. 编辑后按影响面运行 `make fix-file FILE=<path>`、目标测试和 verify。

## 同步纪律

`.claude` 与 `.codex` 是长期并存的镜像资产。任意一边修改 rules、workflows、templates 或入口文件后，必须运行：

- `make sync-agent-governance`：从 Claude 侧同步到 Codex 侧。
- `python3 scripts/sync-agent-governance.py --from codex`：从 Codex 侧同步回 Claude 侧。
- `make verify-agent-governance-sync`：确认两边无漂移。
"""


def read_bytes(path: pathlib.Path) -> bytes:
    return path.read_bytes()


def same_file(left: pathlib.Path, right: pathlib.Path) -> bool:
    return left.exists() and right.exists() and filecmp.cmp(left, right, shallow=False)


def copy_file(src: pathlib.Path, dst: pathlib.Path) -> None:
    dst.parent.mkdir(parents=True, exist_ok=True)
    shutil.copy2(src, dst)


def sync_tree(src: pathlib.Path, dst: pathlib.Path) -> None:
    if dst.exists():
        shutil.rmtree(dst)
    if src.exists():
        shutil.copytree(src, dst)


def claude_entry_files() -> list[pathlib.Path]:
    return sorted(
        path
        for path in ROOT.glob("**/CLAUDE.md")
        if ".git" not in path.parts and ".codex" not in path.parts
    )


def codex_entry_for(claude_file: pathlib.Path) -> pathlib.Path:
    return claude_file.with_name("AGENTS.md")


def sync_entries(direction: str) -> None:
    if direction == "claude":
        for claude_file in claude_entry_files():
            copy_file(claude_file, codex_entry_for(claude_file))
        return

    for agents_file in sorted(
        path
        for path in ROOT.glob("**/AGENTS.md")
        if ".git" not in path.parts and ".codex" not in path.parts
    ):
        copy_file(agents_file, agents_file.with_name("CLAUDE.md"))


def sync_side(direction: str) -> None:
    if direction == "claude":
        sync_entries(direction)
        sync_tree(CLAUDE_DIR / "rules", CODEX_RULES_DIR)
        sync_tree(CLAUDE_DIR / "commands", CODEX_WORKFLOWS_DIR)
        sync_tree(CLAUDE_DIR / "templates", CODEX_TEMPLATES_DIR)
    else:
        sync_entries(direction)
        sync_tree(CODEX_RULES_DIR, CLAUDE_DIR / "rules")
        sync_tree(CODEX_WORKFLOWS_DIR, CLAUDE_DIR / "commands")
        sync_tree(CODEX_TEMPLATES_DIR, CLAUDE_DIR / "templates")
    write_skill_file()


def write_skill_file() -> None:
    SKILL_FILE.parent.mkdir(parents=True, exist_ok=True)
    current = SKILL_FILE.read_text() if SKILL_FILE.exists() else ""
    if current != SKILL_BODY:
        SKILL_FILE.write_text(SKILL_BODY)


def compare_trees(left: pathlib.Path, right: pathlib.Path) -> list[str]:
    errors: list[str] = []
    left_files = {path.relative_to(left) for path in left.glob("**/*") if path.is_file()} if left.exists() else set()
    right_files = {path.relative_to(right) for path in right.glob("**/*") if path.is_file()} if right.exists() else set()
    for rel in sorted(left_files - right_files):
        errors.append(f"missing Codex mirror: {right / rel}")
    for rel in sorted(right_files - left_files):
        errors.append(f"extra Codex mirror without Claude source: {right / rel}")
    for rel in sorted(left_files & right_files):
        if not same_file(left / rel, right / rel):
            errors.append(f"agent governance drift: {left / rel} != {right / rel}")
    return errors


def check_entries() -> list[str]:
    errors: list[str] = []
    claude_files = set(claude_entry_files())
    agent_files = {
        path
        for path in ROOT.glob("**/AGENTS.md")
        if ".git" not in path.parts and ".codex" not in path.parts
    }

    for claude_file in sorted(claude_files):
        agents_file = codex_entry_for(claude_file)
        if not agents_file.exists():
            errors.append(f"missing Codex entry mirror: {agents_file}")
        elif read_bytes(claude_file) != read_bytes(agents_file):
            errors.append(f"agent entry drift: {claude_file} != {agents_file}")

    expected_agents = {codex_entry_for(path) for path in claude_files}
    for agents_file in sorted(agent_files - expected_agents):
        errors.append(f"extra Codex entry without Claude source: {agents_file}")
    return errors


def check_skill_file() -> list[str]:
    if not SKILL_FILE.exists():
        return [f"missing Codex skill file: {SKILL_FILE}"]
    if SKILL_FILE.read_text() != SKILL_BODY:
        return [f"Codex skill file is not generated from scripts/sync-agent-governance.py: {SKILL_FILE}"]
    return []


def check_sync() -> list[str]:
    return (
        check_entries()
        + compare_trees(CLAUDE_DIR / "rules", CODEX_RULES_DIR)
        + compare_trees(CLAUDE_DIR / "commands", CODEX_WORKFLOWS_DIR)
        + compare_trees(CLAUDE_DIR / "templates", CODEX_TEMPLATES_DIR)
        + check_skill_file()
    )


def main() -> int:
    parser = argparse.ArgumentParser(description="同步 Claude 与 Codex Agent 治理资产")
    group = parser.add_mutually_exclusive_group(required=True)
    group.add_argument("--from", dest="source", choices=("claude", "codex"), help="选择同步来源")
    group.add_argument("--check", action="store_true", help="只检查双边是否同步")
    args = parser.parse_args()

    if args.check:
        errors = check_sync()
        if errors:
            for error in errors:
                print(error, file=sys.stderr)
            return 1
        return 0

    sync_side(args.source)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
