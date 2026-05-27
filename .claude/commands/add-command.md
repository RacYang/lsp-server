---
description: 新增 Claude Code 命令。用于为新的任务类型增加标准流程并接入 CLAUDE.md 路由。
---

# 新增命令

## When to use

当出现重复任务流程，且 CLAUDE.md 路由中缺少稳定入口时使用。

## Inputs

- 命令名称：使用小写短横线，最终文件名为 `.claude/commands/<name>.md`。
- 生命周期阶段与覆盖矩阵单元。
- 关联 ADR、rules 与 templates。

## Steps

1. **确认无重叠覆盖**：检索现有 rules/commands/ADR，确认无已有资产覆盖此场景；列出最近相关资产差异（ADR-0049）。
2. 在 `.claude/commands/` 新建 `<name>.md`，frontmatter 中声明 `description`。
3. 保持四个锚点：`## When to use`、`## Inputs`、`## Steps`、`## Verify`。
4. 更新 `docs/agent-governance/coverage-matrix.md` 与 `CLAUDE.md` 的任务路由表。
5. 不在命令中复述规则的 enforcer 或负例细节。

## Verify

- 运行 `make verify-skeleton`。
- 运行 `make verify-doc-links`。
