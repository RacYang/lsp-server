---
description: 退役硬约束。用于删除或合并不再需要的 rule、enforcer 与负例。
---

# 退役规则

## When to use

当硬约束被替代、合并或不再适用，需要删除 `.claude/rules/*.md` 时使用。

## Inputs

- 目标 rule。
- 替代 ADR、rule 或删除理由。
- 关联 enforcer 与负例。

## Steps

1. **输出全量影响面**：列出所有引用方（代码、文档、ADR、config）；确认无用户可见不兼容影响；声明迁移窗口（ADR-0049）。
2. 先确认 ADR 已记录退役原因。
3. 查找 `CLAUDE.md`、`docs/agent-governance/coverage-matrix.md`、commands、templates 中的引用。
4. 删除或迁移 rule、enforcer 与负例。
5. 更新覆盖矩阵，确保没有悬空引用。

## Verify

- 运行 `make verify-meta`。
- 运行 `make verify-doc-links`。
