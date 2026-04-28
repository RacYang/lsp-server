---
name: add-runtime-knob
description: 新增运行时参数。用于扩展进程 YAML、internal/config.Config、默认值、测试与 ADR-0022 约束下的 runtime.* 配置。
---

# 新增运行时参数

## When to use

当需要新增 gate、lobby、room、Redis、PostgreSQL 或 actor 相关运行时参数，并且该参数属于业务运行配置而非治理 SSOT 时使用。

## Inputs

- 参数路径：例如 `runtime.room.mailbox_capacity`。
- 默认值与单位：必须说明行为含义与零值策略。
- 使用点：读取配置的服务或存储组件。

## Steps

1. 确认参数属于运行时配置，不放入 `.build/config.yaml`。
2. 更新 `internal/config/config.go` 的结构体、默认值与解析逻辑。
3. 更新 `configs/dev.yaml` 或对应进程配置示例。
4. 在消费侧只读取结构化配置，不散落环境变量或 magic number。
5. 补 `internal/config` 单测与消费侧边界测试。
6. 更新相关文档或 ADR-0022 的引用说明。

## Verify

- 运行 `go test ./internal/config ./internal/...` 中相关包。
- 运行 `make verify-fast`。
