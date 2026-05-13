---
description: 退役模板。用于删除、合并或替换 .cursor/templates 下的骨架资产。
---

# 退役模板

## When to use

当代码骨架、注释样板或文件头被新模板替代时使用。

## Inputs

- 目标 template。
- 替代 template。
- 受影响的 commands 与覆盖矩阵单元。

## Steps

1. 更新引用该 template 的 commands 与 `CLAUDE.md` 路由。
2. 更新覆盖矩阵和相关 manifest。
3. 删除模板目录，或保留迁移说明。
4. 确认 verify-skeleton 不再引用旧 manifest。

## Verify

- 运行 `make verify-doc-links`。
- 运行 `make verify-skeleton`。
