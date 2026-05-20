---
description: 治理资产自身须通过形态、计数与锚点校验
alwaysApply: true
---

# 治理元约束

- CLAUDE.md 行数不超过 SSOT `governance.entry_md_max_lines`，且必须包含 `entry_md_required_anchors` 中声明的全部锚点。
- `.claude/rules/`、`.claude/commands/`、`.claude/templates/` 的数量不得超过 SSOT `governance` 节中各自的 `*_max_count`。
- 每条规则必须在 frontmatter 中声明 `description`，并在正文中以 `**ADR**`、`**Enforcer**`、`**负例**` 引用三元组。

---

- **ADR**：`docs/adr/0042-agent-governance.md`
- **Enforcer**：`scripts/verify-meta.py`
- **负例**：`.build/negatives/rule_shape_missing_enforcer.mdc.neg`
