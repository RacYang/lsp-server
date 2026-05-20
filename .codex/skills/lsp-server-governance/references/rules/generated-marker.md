---
description: 派生产物必须声明生成来源并禁止手改
globs: ["**/gen/**/*.go"]
---

# 生成物标记

- 派生 Go 文件必须包含 `Code generated ... DO NOT EDIT.`。
- 修改源文件后运行生成命令，不手改派生产物。

---

- **ADR**：`docs/adr/0000-engineering-charter.md`
- **Enforcer**：`scripts/verify-source-shape.py`
- **负例**：`.build/negatives/generated_marker_missing.go.neg`
