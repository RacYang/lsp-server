---
description: 已存在的日志边界文件须向 Context 注入追踪字段
globs: ["**/*.go"]
---

# 日志上下文边界

- 传输入口（gRPC handler）、actor 消息循环、CLI 命令入口与定时器回调属于"日志上下文边界"。
- 边界文件必须在 Context 中通过 `logx.WithTraceID`、`logx.WithUserID` 与 `logx.WithRoomID` 注入追踪字段。
- 非边界业务代码不得手写 `trace_id`、`user_id`、`room_id` 字面量——由门面从 Context 自动提取注入。
- 边界文件由 `scripts/verify-log-boundaries.py` 的检测逻辑判定，不得在检测覆盖范围外自创注入点。

---

- **ADR**：`docs/adr/0006-logging-system-and-facade.md`
- **Enforcer**：`scripts/verify-log-boundaries.py`
- **负例**：`.build/negatives/lang_log_missing_boundary.go.neg`、`.build/negatives/lang_log_literal_required_key.go.neg`
