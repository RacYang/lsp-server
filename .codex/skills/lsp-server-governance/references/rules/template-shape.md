---
description: 模板目录必须提供 manifest 并声明校验钩子
globs: [".claude/templates/**/manifest.yaml"]
---

# Template 形态

- 每个模板目录必须提供 `manifest.yaml`。
- manifest 必须声明用途、适用范围、覆盖矩阵单元和 verify hook。

---

- **ADR**：`docs/adr/0042-agent-governance.md`
- **Enforcer**：`scripts/verify-skeleton.py`
- **负例**：`.build/negatives/template_shape_missing_required.yaml.neg`
