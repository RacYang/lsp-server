---
title: 治理体系迁移至 .claude/ 唯一事实源
status: accepted
date: 2026-05-13
---

# ADR-0046 治理体系迁移至 .claude/ 唯一事实源

## 状态

已采纳。

## 背景

ADR-0042 以 Cursor IDE 的路径约定（`.cursor/rules/`、`.cursor/skills/`、`.cursor/templates/`、`AGENTS.md`）定义了 5+1 治理资产边界。项目随后从 Cursor IDE 迁移至 Claude Code，创建了 `.claude/` 适配层（`.claude/rules/`、`.claude/commands/`、`CLAUDE.md`），但迁移未完成——harness（verify 脚本、SSOT、schema）仍校验 `.cursor/` 路径，模板层完全缺失，两条规则和一条命令遗漏，AGENTS.md 与 CLAUDE.md 内容重叠形成双入口竞争。

这不是简单的格式差异，而是治理体系在工具迁移过程中出现的结构性断层：实际运行的是 `.claude/` 体系，但校验的是 `.cursor/` 体系。

## 决策

1. **`.claude/` 为治理资产唯一事实源**。`.cursor/` 完整废弃并移除。
2. **物理位置重映射**：`CLAUDE.md` 替代 `AGENTS.md` 为入口路由；`.claude/rules/`、`.claude/commands/`、`.claude/templates/` 分别替代 `.cursor/rules/`、`.cursor/skills/`、`.cursor/templates/`。
3. **术语对齐**："skill" 统一改为 "command"，反映 Claude Code 原生概念。`skill-shape` 规则改名为 `command-shape`，`add-skill` 命令改名为 `add-command`。
4. **规则格式**：`.claude/rules/*.md` 使用 Claude Code 原生 frontmatter（`description` + `globs`/`alwaysApply`），ADR/enforcer/负例 引用保留在正文参考节中以供 harness 解析。取消 `kind: constraint` frontmatter 要求（列为 `.claude/rules/` 即隐含约束语义）。
5. **Harness 重定向**：所有 verify 脚本、schema、SSOT（`.build/config.yaml`）中 `agents_md_*`→`entry_md_*`、`skills_*`→`commands_*`、`.cursor/`→`.claude/`、`.mdc`→`.md`。
6. **补齐缺口**：迁移全部 11 个模板目录至 `.claude/templates/`，新增 `error-handling` 与 `log-field-literal-context` 两条规则，新增 `add-template` 命令。
7. **ADR-0042 的路径引用由本文档替代**。ADR-0042 的 5+1 架构原则、职责边界和禁忌规则继续有效，仅物理路径以本文档为准。

## 后果

- `.cursor/` 目录整体移除，无遗留引用。
- `AGENTS.md` 删除，`CLAUDE.md` 成为唯一入口路由。
- SSOT governance 节使用 `entry_md_max_lines`、`commands_max_count`。
- `rules.schema.json` 仅校验 `description` + `globs`/`alwaysApply`。
- `manifest.schema.json` 使用 `related_commands` 替代 `related_skills`。
- 覆盖矩阵全部路径更新为 `.claude/`。
- 治理体系实现真正的单一事实源，harness 校验的即为 Claude Code 实际运行的。

## 相关

- [ADR-0042](0042-agent-governance.md) Agent 治理体系职责边界
- [ADR-0000](0000-engineering-charter.md) 工程宪章
