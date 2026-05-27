---
description: 经日志门面输出的 message 以中文为主并符合字段治理
globs: ["internal/**/*.go", "cmd/**/*.go", "pkg/**/*.go"]
---

# 日志 message 语言

- 经 `logx` 门面输出的日志 message 必须以简体中文撰写。
- 日志结构化字段的命名规范见 `log-field-naming.md`；PII 脱敏要求见 `log-pii-field.md`。

---

- **ADR**：`docs/adr/0004-language-and-writing-policy.md`
- **Enforcer**：`scripts/verify-log-calls.py`
- **负例**：`.build/negatives/lang_log_english_message.go.neg`
