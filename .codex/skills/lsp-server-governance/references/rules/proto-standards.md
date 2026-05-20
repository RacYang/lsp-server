---
description: Proto 文件必须遵循 buf lint 约束并为 breaking 检查预留兼容性
globs: ["api/proto/**/*.proto"]
---

# Proto 规范

- 使用稳定的包名与 `go_package` 选项。
- 客户端契约与 RPC 契约分树维护。

---

- **ADR**：`docs/adr/0000-engineering-charter.md`
- **Enforcer**：`buf.yaml`
- **负例**：`.build/negatives/proto_invalid_package.proto.neg`
