---
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
