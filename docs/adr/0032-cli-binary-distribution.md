# ADR-0032 lsp-cli 二进制分发（已退役）

状态：**已退役**（2026-05-28）

## 退役原因

- `cmd/cli`（TUI 客户端）已随前端清理一并删除。
- 相关规则 `.claude/rules/cli-release-targets.md`、脚本 `scripts/verify-cli-release-targets.py` 以及 `.goreleaser.yaml` 均已移除。
- 本 ADR 不再有执行约束，保留编号以维持序列连续性。
