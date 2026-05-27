---
description: 任何修复/新增前必须全系统梳理并从最优解出发，禁止补丁式修复
globs: ["**/*.go", "**/*.proto"]
---

# 零债务重构策略

- 遇到问题时，第一步是**全系统梳理**：找出所有涉及该职责的文件、接口、调用链，画出当前状态与最优目标状态之间的 diff。
- 禁止在错误的层做修复：如果根因在 A 层，不得在 B 层加 guard、重试或默认值来屏蔽。
- 判断标准：若修复后代码行数没有减少（或新增超过删除），大概率是在加补丁而非解决问题。
- 每次修复必须回答：**谁拥有这个职责**？将修复落在职责所有者的代码路径上，不在调用方修复。
- 明确区分"临时缓解"与"完成修复"：临时缓解必须留 `TODO(debt): <回收条件>` 注释，不得作为最终提交。
- 重构不怕范围大，怕的是方向错。重构前在 context 中明确写出目标状态（接口、数据流、职责边界），再动代码。

---

- **ADR**：`docs/adr/0043-root-cause-fix-policy.md`、`docs/adr/0049-zero-debt-refactor-strategy.md`
- **Enforcer**：`scripts/verify-source-shape.py`
- **负例**：`.build/negatives/zero_debt_patch_guard.go.neg`
