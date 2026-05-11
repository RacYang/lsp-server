# Agent 治理覆盖矩阵

本文档是 Agent 治理体系的活文档，记录常见任务应使用的 skill、template、rule 与 ADR。长期职责边界见 [ADR-0042](../adr/0042-agent-governance.md)。

## 生命周期

| 阶段 | 目标 | 入口 |
|------|------|------|
| design | 形成长期决策、弃用路径或治理原则 | `.cursor/skills/add-adr/SKILL.md`、`.cursor/skills/deprecate-feature/SKILL.md` |
| build | 构造代码、协议、配置或玩法能力 | `.cursor/skills/scaffold-package/SKILL.md`、`.cursor/skills/add-pb-message/SKILL.md`、`.cursor/skills/add-mahjong-rule/SKILL.md` |
| verify | 增加测试、压测、夹具与回归验证 | `.cursor/skills/add-table-test/SKILL.md`、`.cursor/skills/add-integration-test/SKILL.md`、`.cursor/skills/add-bench-scenario/SKILL.md` |
| operate | 增加观测、部署覆盖或运行手册 | `.cursor/skills/add-metric/SKILL.md`、`.cursor/skills/add-log-boundary/SKILL.md`、`.cursor/skills/add-deploy-overlay/SKILL.md` |
| evolve | 升级、退役、根因修复或维护治理资产 | `.cursor/skills/fix-bug/SKILL.md`、`.cursor/skills/update-ssot/SKILL.md`、`.cursor/skills/add-constraint/SKILL.md` |

## 架构层

| 层 | 构造入口 | 默认模板 | 关键约束 |
|----|----------|----------|----------|
| gate | `.cursor/skills/scaffold-package/SKILL.md` | `.cursor/templates/code/go-package/manifest.yaml` | `.cursor/rules/architecture-boundaries.mdc` |
| lobby | `.cursor/skills/scaffold-package/SKILL.md` | `.cursor/templates/code/go-service/manifest.yaml` | `.cursor/rules/architecture-boundaries.mdc` |
| room | `.cursor/skills/scaffold-package/SKILL.md` | `.cursor/templates/code/go-actor/manifest.yaml` | `.cursor/rules/architecture-boundaries.mdc` |
| mahjong | `.cursor/skills/add-mahjong-rule/SKILL.md` | `.cursor/templates/code/go-package/manifest.yaml` | `.cursor/rules/mahjong-rule.mdc` |
| api | `.cursor/skills/add-pb-message/SKILL.md` | `.cursor/templates/code/proto-message/manifest.yaml` | `.cursor/rules/proto-standards.mdc` |
| persistence | `.cursor/skills/scaffold-package/SKILL.md` | `.cursor/templates/code/go-repository/manifest.yaml` | `.cursor/rules/redis-keys-naming.mdc` |
| observability | `.cursor/skills/add-metric/SKILL.md` | `.cursor/templates/doc/runbook/manifest.yaml` | `.cursor/rules/metrics-naming.mdc` |
| cli | `.cursor/skills/scaffold-package/SKILL.md` | `.cursor/templates/code/go-cli-command/manifest.yaml` | `.cursor/rules/cli-release-targets.mdc` |

## 横切关注

| 关注点 | skill | template | rule | ADR |
|--------|-------|----------|------|-----|
| ADR | `.cursor/skills/add-adr/SKILL.md` | `.cursor/templates/doc/adr/manifest.yaml` | `.cursor/rules/docs-chinese.mdc` | `docs/adr/0000-engineering-charter.md` |
| 硬约束 | `.cursor/skills/add-constraint/SKILL.md` | `.cursor/templates/doc/rule/manifest.yaml` | `.cursor/rules/rule-shape.mdc` | `docs/adr/0042-agent-governance.md` |
| skill | `.cursor/skills/add-skill/SKILL.md` | `.cursor/templates/doc/skill/manifest.yaml` | `.cursor/rules/skill-shape.mdc` | `docs/adr/0042-agent-governance.md` |
| template | `.cursor/skills/add-template/SKILL.md` | `.cursor/templates/doc/template/manifest.yaml` | `.cursor/rules/template-shape.mdc` | `docs/adr/0042-agent-governance.md` |
| 注释 | `.cursor/skills/scaffold-package/SKILL.md` | `.cursor/templates/comment/exported-func/manifest.yaml` | `.cursor/rules/code-comments-chinese.mdc` | `docs/adr/0005-comment-system.md` |
| 根因修复 | `.cursor/skills/fix-bug/SKILL.md` | `.cursor/templates/code/go-table-test/manifest.yaml` | `.cursor/rules/test-layout.mdc` | `docs/adr/0043-root-cause-fix-policy.md` |
| 错误处理 | `.cursor/skills/fix-bug/SKILL.md` | `.cursor/templates/code/go-table-test/manifest.yaml` | `.cursor/rules/error-handling.mdc` | `docs/adr/0000-engineering-charter.md` |
| 日志 | `.cursor/skills/add-log-boundary/SKILL.md` | `.cursor/templates/doc/runbook/manifest.yaml` | `.cursor/rules/no-direct-logging.mdc` | `docs/adr/0006-logging-system-and-facade.md` |
| 指标 | `.cursor/skills/add-metric/SKILL.md` | `.cursor/templates/doc/runbook/manifest.yaml` | `.cursor/rules/metrics-naming.mdc` | `docs/adr/0019-observability-metrics.md` |
| Proto | `.cursor/skills/add-pb-message/SKILL.md` | `.cursor/templates/code/proto-message/manifest.yaml` | `.cursor/rules/proto-baseline.mdc` | `docs/adr/0012-proto-baseline-and-versioning.md` |
| Git | `.cursor/skills/update-ssot/SKILL.md` | `.cursor/templates/doc/runbook/manifest.yaml` | `.cursor/rules/git-hooks-parity.mdc` | `docs/adr/0007-git-workflow-policy.md` |

## 覆盖规则

- 矩阵中的路径必须真实存在。
- 每个新增 skill 必须能落到一个生命周期阶段。
- 每个新增 template 必须声明 `coverage_cell`。
- 每个新增 constraint rule 必须有 ADR、enforcer 与负例。
- 如果矩阵提到的资产尚未创建，执行对应阶段时必须优先补齐。
