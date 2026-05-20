---
description: 禁止引入被 SSOT 明确拒绝的游戏服务器框架依赖
alwaysApply: true
---

# 依赖策略

仓库可以使用聚焦的单点库，但不得引入 `.build/config.yaml` 中 `deps.denylist` 列出的游戏服务器框架。

该规则落实 ADR-0001：房间编排、规则分发、路由与协议处理必须由本仓库自有代码承担。

---

- **ADR**：`docs/adr/0001-self-built-vs-framework.md`
- **Enforcer**：`scripts/dep-guard.sh`
- **负例**：`.build/negatives/deps_forbidden_dependency.go.neg`
