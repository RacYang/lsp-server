---
description: OTel 日志桥接默认关闭，以 build tag 预留路径，不得默认启用
globs: ["internal/**/*.go", "cmd/**/*.go", "pkg/**/*.go"]
---

# OpenTelemetry 日志桥接

- OTel 日志桥接当前默认关闭，仅通过 `otel_log_bridge` build tag 预留 trace/span 注入路径。
- 禁止在未完成 ADR 评估前将 OTel 桥接设为默认开启。
- 如需启用，须先通过 ADR 明确性能影响、采样策略与供应商绑定风险。

---

- **ADR**：`docs/adr/0035-otel-logs-bridge.md`
- **Enforcer**：`scripts/verify-log-calls.py`
- **负例**：`.build/negatives/lang_log_unknown_field_key.go.neg`
