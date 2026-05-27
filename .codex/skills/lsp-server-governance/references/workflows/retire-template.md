---
description: 退役模板。用于删除、合并或替换 .claude/templates 下的骨架资产。
---

# 退役模板

## When to use

当代码骨架、注释样板或文件头被新模板替代时使用。

## Inputs

- 目标 template。
- 替代 template。
- 受影响的 commands 与覆盖矩阵单元。

## Steps

1. **输出全量影响面**：列出所有引用方（代码、文档、ADR、config）；确认无用户可见不兼容影响；声明迁移窗口（ADR-0049）。
2. 更新引用该 template 的 commands 与 `CLAUDE.md` 路由。
3. 更新覆盖矩阵和相关 manifest。
4. 删除模板目录，或保留迁移说明。
5. 确认 verify-skeleton 不再引用旧 manifest。

## Verify

- 运行 `make verify-doc-links`。
- 运行 `make verify-skeleton`。
