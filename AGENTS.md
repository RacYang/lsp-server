# AGENTS

本文件是 agent 入口路由；长期决策见 `docs/adr/`，硬约束见 `.cursor/rules/`，任务流程见 `.cursor/skills/`，骨架样板见 `.cursor/templates/`。

## 总纪律

1. 治理源变更先改 `.build/config.yaml`；派生产物通过 `make generate` 更新。
2. 实质性改动完成前运行与影响面匹配的 verify，默认最终运行 `make verify`。
3. 不绕过 Git hook，不提交密钥、私有配置、临时产物或无关改动。
4. 麻将规则逻辑与传输、会话、存储、集群和 app 层保持隔离。
5. 查问题和修问题必须根因优先；拒绝只屏蔽症状的补丁式修复。

## 任务路由

| 想做的事 | 先看 |
|----------|------|
| 新增或修订长期决策 | `.cursor/skills/add-adr/SKILL.md` |
| 新增硬约束、enforcer 或负例 | `.cursor/skills/add-constraint/SKILL.md` |
| 修改治理阈值、工具版本或白名单 | `.cursor/skills/update-ssot/SKILL.md` |
| 新增 Go 包、handler、service、actor 或 CLI 子命令 | `.cursor/skills/scaffold-package/SKILL.md` |
| 新增 proto 消息或 RPC 契约 | `.cursor/skills/add-pb-message/SKILL.md` |
| 新增麻将规则、番种或 YAML 夹具 | `.cursor/skills/add-mahjong-rule/SKILL.md`、`.cursor/skills/add-fan-type/SKILL.md`、`.cursor/skills/mahjong-test-case/SKILL.md` |
| 新增测试、集成链路或压测场景 | `.cursor/skills/add-table-test/SKILL.md`、`.cursor/skills/add-integration-test/SKILL.md`、`.cursor/skills/add-bench-scenario/SKILL.md` |
| 新增指标、日志边界或部署覆盖 | `.cursor/skills/add-metric/SKILL.md`、`.cursor/skills/add-log-boundary/SKILL.md`、`.cursor/skills/add-deploy-overlay/SKILL.md` |
| 查问题、修 bug、升级依赖、移动 proto 基线或弃用功能 | `.cursor/skills/fix-bug/SKILL.md`、`.cursor/skills/bump-dependency/SKILL.md`、`.cursor/skills/bump-proto-baseline/SKILL.md`、`.cursor/skills/deprecate-feature/SKILL.md` |
| 新增或退役 skill/template/rule | `.cursor/skills/add-skill/SKILL.md`、`.cursor/skills/add-template/SKILL.md`、`.cursor/skills/retire-rule/SKILL.md`、`.cursor/skills/retire-skill/SKILL.md`、`.cursor/skills/retire-template/SKILL.md` |

## 导航

- 工程宪章与 SSOT：`docs/adr/0000-engineering-charter.md`
- Git 工作流：`docs/adr/0007-git-workflow-policy.md`
- 注释体系：`docs/adr/0005-comment-system.md`
- 日志体系：`docs/adr/0006-logging-system-and-facade.md`、`docs/adr/0033-logging-sampling-and-pii-redaction.md`、`docs/adr/0034-logging-dynamic-level-control.md`、`docs/adr/0035-otel-logs-bridge.md`
- 麻将规则组合与房间契约：`docs/adr/0040-composable-mahjong-rule-capabilities.md`、`docs/RULE-ENGINE.md`、`docs/ROOM-FSM.md`、`docs/cli-tui-backend-gaps.md`
- Agent 治理体系：`docs/adr/0042-agent-governance.md`、`docs/agent-governance/coverage-matrix.md`
- 根因修复策略：`docs/adr/0043-root-cause-fix-policy.md`

## 边界声明

`AGENTS.md` 只做入口，不复述 `.cursor/rules/*.mdc` 的硬约束细节。扩展本体系时同步更新覆盖矩阵，并运行 `make verify-meta`、`make verify-skeleton` 与相关 verify 目标。
