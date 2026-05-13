---
description: 敏感字段不得以原始键进入日志调用
globs: ["**/*.go"]
---

# 日志敏感字段

`token`、`password`、`email` 等敏感键不得在业务日志调用中直接出现；如确需关联，应使用脱敏派生字段。

---

- **ADR**：`docs/adr/0033-logging-sampling-and-pii-redaction.md`
- **Enforcer**：`scripts/verify-log-calls.py`
- **负例**：`.build/negatives/lang_log_pii_field_key.go.neg`
