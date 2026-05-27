---
description: 退役任务命令。用于删除、合并或重命名不再需要的 .claude/commands/*.md。
---

# 退役命令

## When to use

当某个 workflow 被合并、失效或改由其他命令覆盖时使用。

## Inputs

- 目标命令文件名。
- 替代命令或删除理由。
- 覆盖矩阵中的单元。

## Steps

1. **输出全量影响面**：列出所有引用方（代码、文档、ADR、config）；确认无用户可见不兼容影响；声明迁移窗口（ADR-0049）。
2. 先在覆盖矩阵中确定替代入口。
3. 更新 `CLAUDE.md` 的任务路由表和相关 template manifest。
4. 删除旧命令文件或保留迁移说明。
5. 确认没有文档继续指向旧路径。

## Verify

- 运行 `make verify-doc-links`。
- 运行 `make verify-skeleton`。
