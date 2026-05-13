---
description: 新增 Go 包必须提供中文包注释骨架
globs: ["**/doc.go"]
---

# 包注释

- 新增 Go 包应包含 `doc.go`。
- 包注释说明职责、边界和禁止事项，正文以中文为主。

---

- **ADR**：`docs/adr/0005-comment-system.md`
- **Enforcer**：`scripts/verify-source-shape.py`
- **负例**：`.build/negatives/package_doc_missing.go.neg`
