# Agent 治理覆盖矩阵

本文档是 Agent 治理体系的活文档，记录常见任务应使用的 command、template、rule 与 ADR。长期职责边界见 [ADR-0042](../adr/0042-agent-governance.md)。

Claude 与 Codex 治理资产长期双边保留：`.claude/` 是 Claude 入口，`.codex/skills/lsp-server-governance/` 是 Codex 入口。任意一边修改后，必须运行 `make sync-agent-governance` 或 `python3 scripts/sync-agent-governance.py --from codex`，并通过 `make verify-agent-governance-sync`。

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
| Git 合并 | — | — | `.claude/rules/git-merge-policy.md` | `docs/adr/0007-git-workflow-policy.md` |
| Git 强推 | — | — | `.claude/rules/git-force-push-policy.md` | `docs/adr/0007-git-workflow-policy.md` |
| 提交签名 | — | — | `.claude/rules/git-commit-signing.md` | `docs/adr/0007-git-workflow-policy.md` |
| 阶段门控 | — | — | `.claude/rules/stage-gates.md` | `docs/adr/0000-engineering-charter.md` |
| 工具版本 | — | — | `.claude/rules/tool-versions.md` | `docs/adr/0000-engineering-charter.md` |
| 数据库迁移 | — | — | `.claude/rules/db-migration-policy.md` | `docs/adr/0000-engineering-charter.md` |
| 可观测规则 | — | — | `.claude/rules/observability-rules.md` | `docs/adr/0019-observability-metrics.md` |
| 治理元约束 | — | — | `.claude/rules/governance-meta.md` | `docs/adr/0042-agent-governance.md` |
| 配置完整性 | — | — | `.claude/rules/config-integrity.md` | `docs/adr/0000-engineering-charter.md` |
| 文档链接 | — | — | `.claude/rules/doc-link-integrity.md` | `docs/adr/0042-agent-governance.md` |
| 治理文档格式 | — | — | `.claude/rules/agent-first-docs.md` | `docs/adr/0048-agent-first-governance-format.md` |
| 零债务重构 | `/fix-bug`、`/scaffold-package` | — | `.claude/rules/zero-debt-refactor.md` | `docs/adr/0049-zero-debt-refactor-strategy.md` |

## 覆盖规则

- 矩阵中的路径必须真实存在。
- 每个新增 command 必须能落到一个生命周期阶段。
- 每个新增 template 必须声明 `coverage_cell`。
- 每个新增 constraint rule 必须有 ADR、enforcer 与负例。
- `.claude/rules`、`.claude/commands`、`.claude/templates` 必须与 `.codex/skills/lsp-server-governance/` 下对应镜像保持同步。
- 如果矩阵提到的资产尚未创建，执行对应阶段时必须优先补齐。
