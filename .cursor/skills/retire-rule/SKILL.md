---
name: retire-rule
description: 退役硬约束。用于删除或合并不再需要的 rule、enforcer 与负例。
---

# 退役规则

## When to use

当硬约束被替代、合并或不再适用，需要删除 `.cursor/rules/*.mdc` 时使用。

## Inputs

- 目标 rule。
- 替代 ADR、rule 或删除理由。
- 关联 enforcer 与负例。

## Steps

1. 先确认 ADR 已记录退役原因。
2. 查找 AGENTS、coverage matrix、skills、templates 中的引用。
3. 删除或迁移 rule、enforcer 与负例。
4. 更新覆盖矩阵，确保没有悬空引用。

## Verify

- 运行 `make verify-meta`。
- 运行 `make verify-doc-links`。
