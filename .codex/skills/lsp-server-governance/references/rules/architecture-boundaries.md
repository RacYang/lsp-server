---
description: 分层依赖必须受架构边界约束
globs: ["internal/**/*.go", "cmd/**/*.go", "pkg/**/*.go"]
---

# 架构边界

- `internal/handler` 不得 import `internal/store`。
- `internal/mahjong` 不得依赖传输、会话、集群或 app 等包。
- 局内 proto 与 TUI 字段应由 `internal/service/room` 的权威状态投影，不在 handler/gateway 中拼业务事实。
- 即使能编译，也拒绝跨层捷径。

---

- **ADR**：`docs/adr/0000-engineering-charter.md`
- **Enforcer**：`.go-arch-lint.yml`
- **负例**：`.build/negatives/arch_handler_imports_store.go.neg`
