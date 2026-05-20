---
description: 单元测试优先采用表驱动骨架
globs: ["**/*_test.go"]
---

# 测试布局

- 新增单元测试优先使用 `tests := []struct` 表驱动骨架。
- 测试名描述业务行为，断言完整对象优于散点字段。

---

- **ADR**：`docs/adr/0042-agent-governance.md`
- **Enforcer**：`scripts/verify-source-shape.py`
- **负例**：`.build/negatives/test_layout_missing_table.go.neg`
