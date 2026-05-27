---
description: 治理文档以 agent 高效解析为首要目标，禁止冗余散文
globs: [".claude/**/*.md", "docs/adr/*.md", "CLAUDE.md"]
---

# Agent 优先文档格式

- 所有治理资产（rules、commands、templates、ADR、CLAUDE.md）以 agent 解析效率为唯一优化目标；人类可读性是副产品，不是目标。
- 使用列表、表格、代码块；禁止解释性散文、背景叙述、"为什么这很重要"式段落。
- 每条 rule 正文不超过 10 行；超出说明规则粒度过粗，应拆分。
- ADR 正文结构固定为：`## 决策`（bullet 列表）+ `## 后果`（bullet 列表）；`## 背景` 仅用于无法在决策中自解释的约束前提，最多 5 行。
- commands 只含 `## Inputs`、`## Steps`、`## Verify` 三节；`## Steps` 为编号列表，每步一个原子操作。
- 禁止在任何治理文档中重复另一文档已有的内容；交叉引用用文件路径表达，不复述原文。

---

- **ADR**：`docs/adr/0048-agent-first-governance-format.md`
- **Enforcer**：`scripts/verify-source-shape.py`
- **负例**：`.build/negatives/rule_shape_agent_first_verbose.mdc.neg`
