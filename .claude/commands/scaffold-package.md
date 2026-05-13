---
description: 新增标准 Go 包骨架。用于按 gate/lobby/room/mahjong/api/persistence/observability/cli 层创建包、doc.go、基础测试与模板化注释。
---

# 新增包骨架

## When to use

当需要新增 handler、service、domain、repository、actor 或 CLI 子命令包时使用。

## Inputs

- 层：`handler`、`service`、`domain`、`repository`、`actor`、`cli`。
- 包路径：必须位于既有架构层允许的目录下。
- 模板：优先复用 `.cursor/templates/code/*/manifest.yaml`。

## Steps

1. 选择与层匹配的 template，并确认 `coverage_cell` 已在覆盖矩阵登记。
2. 创建包目录、`doc.go`、主实现文件与最小测试文件。
3. 保持包注释、导出符号注释与错误处理符合模板。
4. 检查新增 import 不突破 `.go-arch-lint.yml` 的分层边界。

## Verify

- 运行 `make verify-fmt`。
- 运行 `make verify-skeleton`。
- 最后运行包含变更面的 `make verify-fast` 或 `make verify`。
