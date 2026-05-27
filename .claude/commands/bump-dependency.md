---
description: 升级依赖。用于更新 Go 依赖、治理工具版本或生成器版本。
---

# 升级依赖

## When to use

当需要升级 Go 模块依赖、治理工具版本、buf 插件或安全修复依赖时使用。

## Inputs

- 依赖名称、目标版本与升级原因。
- 是否涉及 `.build/config.yaml` 工具版本。
- 风险与回滚方式。

## Steps

1. **输出职责所有者分析**：列出涉及该职责的现有文件、接口、调用链；写出目标状态与当前状态的 diff，确认落点在正确层（ADR-0049）。未完成此步骤不得动代码。
2. 检查依赖是否命中 denylist。
3. 工具版本变更先使用 `/update-ssot` 修改 SSOT。
4. 运行 `go mod tidy` 或对应生成命令。
5. 对受影响模块补充回归测试。

## Verify

- 运行 `make verify-deps`。
- 运行 `make verify-tidy`。
- 最后运行 `make verify`。
