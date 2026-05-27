---
description: Go 导出符号命名不得使用下划线
globs: ["internal/**/*.go", "cmd/**/*.go", "pkg/**/*.go"]
---

# Go 命名

- 导出类型、函数、变量与常量使用 Go 风格驼峰命名。
- 不用下划线伪装分词，业务语义应通过完整英文或拼音表达。

---

- **ADR**：`docs/adr/0042-agent-governance.md`
- **Enforcer**：`scripts/verify-source-shape.py`
- **负例**：`.build/negatives/naming_go_export_underscore.go.neg`
