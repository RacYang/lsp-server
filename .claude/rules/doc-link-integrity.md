---
description: 文档与治理资产中的交叉引用必须指向实际存在的文件
alwaysApply: true
---

# 文档链接完整性

CLAUDE.md、覆盖矩阵、命令文件与规则文件中引用的内部路径（`docs/`、`.claude/`、`.build/`、`scripts/` 等）必须指向实际存在的文件。

新增或重命名资产时必须同步更新所有引用方，避免悬空链接。

---

- **ADR**：`docs/adr/0042-agent-governance.md`
- **Enforcer**：`scripts/verify-doc-links.py`
- **负例**：`.build/negatives/doc_link_broken_reference.md.neg`
