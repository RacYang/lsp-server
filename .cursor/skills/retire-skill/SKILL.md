---
name: retire-skill
description: 退役任务流程。用于删除、合并或重命名不再需要的 SKILL.md。
---

# 退役技能

## When to use

当某个 workflow 被合并、失效或改由其他 skill 覆盖时使用。

## Inputs

- 目标 skill。
- 替代 skill 或删除理由。
- 覆盖矩阵中的单元。

## Steps

1. 先在覆盖矩阵中确定替代入口。
2. 更新 AGENTS.md 路由表和相关 template manifest。
3. 删除旧 skill 或保留迁移说明。
4. 确认没有文档继续指向旧路径。

## Verify

- 运行 `make verify-doc-links`。
- 运行 `make verify-skeleton`。
