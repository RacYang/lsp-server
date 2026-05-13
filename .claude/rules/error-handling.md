---
description: 错误处理必须保留可追溯错误链
globs: ["**/*.go"]
---

# 错误处理

- 包装错误时使用 `%w` 保留错误链。
- 禁止使用字符串拼接或 `%v` 吞掉上游错误语义。

---

- **ADR**：`docs/adr/0000-engineering-charter.md`
- **Enforcer**：`.golangci.yml#{errorlint}`
- **负例**：`.build/negatives/error_handling_missing_wrap.go.neg`
