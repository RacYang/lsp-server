---
description: Claude Code 命令必须使用四锚点结构
globs: [".claude/commands/*.md"]
---

# 命令形态

- 每个命令必须包含 `When to use`、`Inputs`、`Steps`、`Verify` 四个二级标题锚点。
- 命令只描述任务流程，不复述规则的 enforcer 或负例细节。

---

- **ADR**：`docs/adr/0042-agent-governance.md`
- **Enforcer**：`scripts/verify-skeleton.py`
- **负例**：`.build/negatives/command_shape_missing_anchor.md.neg`
