---
description: 新增日志上下文边界。用于在传输、actor、命令入口等边界注入 trace_id、user_id、room_id。
---

# 新增日志边界

## When to use

当新增入口文件、actor 循环、传输 handler 或命令 main，并需要统一日志上下文字段时使用。

## Inputs

- 边界文件路径。
- 可获得的 trace、user、room 字段来源。
- 是否需要扩展 SSOT `logging.context_boundaries`。

## Steps

1. **输出职责所有者分析**：列出涉及该职责的现有文件、接口、调用链；写出目标状态与当前状态的 diff，确认落点在正确层（ADR-0049）。未完成此步骤不得动代码。
2. 在边界处写入 `context.Context`，不要在业务日志调用现场手写核心字段。
3. 通过 `pkg/logx` 门面输出日志。
4. 如新增边界 glob，使用 `/update-ssot` 修改配置。
5. 确认日志 message 以中文为主。

## Verify

- 运行 `make verify-lang`。
- 修改 SSOT 时运行 `make verify-config-schema` 与 `make verify-meta`。
