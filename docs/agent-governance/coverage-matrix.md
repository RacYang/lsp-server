# Agent 治理覆盖矩阵

本文档是 Agent 治理体系的活文档，记录常见任务应使用的 command、template、rule 与 ADR。长期职责边界见 [ADR-0042](../adr/0042-agent-governance.md)。

## 生命周期

| 阶段 | 目标 | 入口 |
|------|------|------|
| design | 形成长期决策、弃用路径或治理原则 | `/add-adr`、`/deprecate-feature` |
| build | 构造代码、协议、配置或玩法能力 | `/scaffold-package`、`/add-pb-message`、`/add-mahjong-rule` |
| verify | 增加测试、压测、夹具与回归验证 | `/add-table-test`、`/add-integration-test`、`/add-bench-scenario` |
| operate | 增加观测、部署覆盖或运行手册 | `/add-metric`、`/add-log-boundary`、`/add-deploy-overlay` |
| evolve | 升级、退役、根因修复或维护治理资产 | `/fix-bug`、`/update-ssot`、`/add-constraint` |

## 架构层

| 层 | 构造入口 | 默认模板 | 关键约束 |
|----|----------|----------|----------|
| gate | `/scaffold-package` | `.claude/templates/code/go-package/manifest.yaml` | `.claude/rules/architecture-boundaries.md` |
| lobby | `/scaffold-package` | `.claude/templates/code/go-service/manifest.yaml` | `.claude/rules/architecture-boundaries.md` |
| room | `/scaffold-package` | `.claude/templates/code/go-actor/manifest.yaml` | `.claude/rules/architecture-boundaries.md` |
| mahjong | `/add-mahjong-rule` | `.claude/templates/code/go-package/manifest.yaml` | `.claude/rules/mahjong-rule.md` |
| api | `/add-pb-message` | `.claude/templates/code/proto-message/manifest.yaml` | `.claude/rules/proto-standards.md` |
| persistence | `/scaffold-package` | `.claude/templates/code/go-repository/manifest.yaml` | `.claude/rules/redis-keys-naming.md` |
| observability | `/add-metric` | `.claude/templates/doc/runbook/manifest.yaml` | `.claude/rules/metrics-naming.md` |
| cli | `/scaffold-package` | `.claude/templates/code/go-cli-command/manifest.yaml` | `.claude/rules/cli-release-targets.md` |

## 横切关注

| 关注点 | command | template | rule | ADR |
|--------|-------|----------|------|-----|
| ADR | `/add-adr` | `.claude/templates/doc/adr/manifest.yaml` | `.claude/rules/docs-chinese.md` | `docs/adr/0000-engineering-charter.md` |
| 硬约束 | `/add-constraint` | `.claude/templates/doc/rule/manifest.yaml` | `.claude/rules/rule-shape.md` | `docs/adr/0042-agent-governance.md` |
| command | `/add-command` | `.claude/templates/doc/command/manifest.yaml` | `.claude/rules/command-shape.md` | `docs/adr/0042-agent-governance.md` |
| template | `/add-template` | `.claude/templates/doc/template/manifest.yaml` | `.claude/rules/template-shape.md` | `docs/adr/0042-agent-governance.md` |
| 注释 | `/scaffold-package` | `.claude/templates/comment/exported-func/manifest.yaml` | `.claude/rules/code-comments-chinese.md` | `docs/adr/0005-comment-system.md` |
| 根因修复 | `/fix-bug` | `.claude/templates/code/go-table-test/manifest.yaml` | `.claude/rules/test-layout.md` | `docs/adr/0043-root-cause-fix-policy.md` |
| 错误处理 | `/fix-bug` | `.claude/templates/code/go-table-test/manifest.yaml` | `.claude/rules/error-handling.md` | `docs/adr/0000-engineering-charter.md` |
| 日志 | `/add-log-boundary` | `.claude/templates/doc/runbook/manifest.yaml` | `.claude/rules/no-direct-logging.md` | `docs/adr/0006-logging-system-and-facade.md` |
| 指标 | `/add-metric` | `.claude/templates/doc/runbook/manifest.yaml` | `.claude/rules/metrics-naming.md` | `docs/adr/0019-observability-metrics.md` |
| Proto | `/add-pb-message` | `.claude/templates/code/proto-message/manifest.yaml` | `.claude/rules/proto-baseline.md` | `docs/adr/0012-proto-baseline-and-versioning.md` |
| Git | `/update-ssot` | `.claude/templates/doc/runbook/manifest.yaml` | `.claude/rules/git-hooks-parity.md` | `docs/adr/0007-git-workflow-policy.md` |

## 覆盖规则

- 矩阵中的路径必须真实存在。
- 每个新增 command 必须能落到一个生命周期阶段。
- 每个新增 template 必须声明 `coverage_cell`。
- 每个新增 constraint rule 必须有 ADR、enforcer 与负例。
- 如果矩阵提到的资产尚未创建，执行对应阶段时必须优先补齐。
