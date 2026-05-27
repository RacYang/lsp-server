---
description: Go 源码注释以中文为主（仅统计注释文本）
globs: ["internal/**/*.go", "cmd/**/*.go", "pkg/**/*.go"]
---

# 注释语言约束

- Go 源码注释（`//` 与 `/* */` 文本，不含代码符号）必须以简体中文为主。
- 注释中出现的英文标识符仅限 SSOT `commenting` 节声明的关键字白名单（如 `ID`、`RPC`、`gRPC`）。
- 中文占比阈值由 `.build/config.yaml` 的 `commenting` 节定义。
- 包注释（`doc.go`）同样受本条约束。

---

- **ADR**：`docs/adr/0004-language-and-writing-policy.md`
- **Enforcer**：`scripts/verify-lang-comments.py`
- **负例**：`.build/negatives/lang_code_english_comment.go.neg`
