---
description: Go 代码必须显式处理错误并保持可追踪的错误链
globs: ["internal/**/*.go", "cmd/**/*.go", "pkg/**/*.go"]
---

# Go 规范

- 不得忽略返回的错误。
- 传播错误时使用 `%w` 包装，保留完整错误链。
- 禁止使用字符串拼接或 `%v` 吞掉上游错误语义。
- 保持函数短小、包依赖显式。

---

- **ADR**：`docs/adr/0000-engineering-charter.md`
- **Enforcer**：`.golangci.yml#{errcheck,errorlint,gocritic}`
- **负例**：`.build/negatives/go_standards_missing_wrap.go.neg`、`.build/negatives/error_handling_missing_wrap.go.neg`
