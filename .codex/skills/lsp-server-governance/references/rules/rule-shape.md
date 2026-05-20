---
description: Claude Code 规则必须在 frontmatter 中声明 description，正文引用 ADR、enforcer 与负例
globs: [".claude/rules/*.md"]
---

# 规则形态

- 每条规则必须在 frontmatter 中声明 `description`。
- 每条规则必须在正文底部「参考」分隔线后列出 **ADR**、**Enforcer** 与 **负例**。
- 规则只承载硬约束，不复述 ADR 的决策背景与 enforcer 的实现细节。

---

- **ADR**：`docs/adr/0042-agent-governance.md`
- **Enforcer**：`scripts/verify-source-shape.py`
- **负例**：`.build/negatives/rule_shape_missing_enforcer.mdc.neg`
