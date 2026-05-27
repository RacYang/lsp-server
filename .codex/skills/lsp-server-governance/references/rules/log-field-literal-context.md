---
description: 禁止业务日志调用手写追踪上下文字段
globs: ["internal/**/*.go", "cmd/**/*.go", "pkg/**/*.go"]
---

# 日志上下文字段

业务日志调用不得手写 `trace_id`、`user_id`、`room_id` 字面量键名。这些字段须通过 `logx.WithTraceID`、`logx.WithUserID`、`logx.WithRoomID` 写入 Context，由门面自动注入。

本规则与 `log-context-boundary.md` 互补：边界规则规定哪些文件必须注入，本规则规定所有文件不得手写字段键名。

---

- **ADR**：`docs/adr/0006-logging-system-and-facade.md`
- **Enforcer**：`scripts/verify-log-calls.py`
- **负例**：`.build/negatives/lang_log_literal_required_key.go.neg`
