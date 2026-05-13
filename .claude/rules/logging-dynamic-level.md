---
description: 日志动态级别当前仅限 Go API，不得开放未鉴权 HTTP 写端点
globs: ["**/*.go"]
---

# 日志动态级别

- 当前 Phase 仅提供 `AtomicLevel` Go API 供内部组件调控日志级别。
- 禁止在未补充 ADR 决策与安全评估前开放 HTTP 写端点或 RPC 接口用于外部日志级别变更。
- `logging.dynamic_level` 的 SSOT 配置变更须与 `/update-ssot` 同步。

---

- **ADR**：`docs/adr/0034-logging-dynamic-level-control.md`
- **Enforcer**：`scripts/verify-log-calls.py`
- **负例**：`.build/negatives/lang_log_unknown_field_key.go.neg`
