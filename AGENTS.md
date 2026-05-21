# lsp-server

麻将游戏服务器，自研网络、房间编排与规则分发，拒绝游戏服务器框架依赖。多服务架构：gate、lobby、room、bot、cli，gRPC 集群层 + proto 客户端契约，Alpha 阶段。

## 不可妥协纪律

1. 治理变更先改 `.build/config.yaml`（SSOT）；派生产物通过 `make generate` 更新，不手改。
2. 实质性改动运行与影响面匹配的 verify，最终默认 `make verify`。
3. 不绕过 Git hook，不提交密钥、私有配置、临时产物或无关改动。
4. 麻将规则逻辑与传输、会话、存储、集群和 app 层保持隔离。
5. 查问题和修问题必须根因优先；拒绝只屏蔽症状的补丁式修复。
6. 每条可执行约束必须有独立 ADR + enforcer + 负例三元组。
7. `.claude` 是治理事实源，`.codex` 是其镜像（供 Codex/Cursor 等工具使用），`AGENTS.md` 是第三方 AI 工具适配入口；任意一侧修改后必须运行 `make sync-agent-governance` 或显式反向同步，并通过 `make verify-agent-governance-sync`。

## 任务路由

| 任务 | 命令 |
|------|------|
| 新增或修订长期决策 | `/add-adr` |
| 新增硬约束、enforcer 或负例 | `/add-constraint` |
| 修改治理阈值、工具版本或白名单 | `/update-ssot` |
| 新增 Go 包、handler、service、actor 或 CLI 子命令 | `/scaffold-package` |
| 新增 proto 消息或 RPC 契约 | `/add-pb-message` |
| 新增麻将规则变体 | `/add-mahjong-rule` |
| 新增番种或计分逻辑 | `/add-fan-type` |
| 新增麻将 YAML 夹具 | `/mahjong-test-case` |
| 新增表驱动单元测试 | `/add-table-test` |
| 新增集成测试 | `/add-integration-test` |
| 新增压测场景 | `/add-bench-scenario` |
| 新增 Prometheus 指标 | `/add-metric` |
| 新增日志上下文边界 | `/add-log-boundary` |
| 新增部署 overlay 或 runbook | `/add-deploy-overlay` |
| 新增运行时参数 | `/add-runtime-knob` |
| 修复缺陷 | `/fix-bug` |
| 升级依赖 | `/bump-dependency` |
| 更新 proto 基线 | `/bump-proto-baseline` |
| 弃用功能或契约 | `/deprecate-feature` |
| 退役规则 | `/retire-rule` |
| 退役命令 | `/retire-command` |
| 退役模板 | `/retire-template` |
| 新增命令 | `/add-command` |
| 新增模板 | `/add-template` |

## 导航

- **工程宪章与 SSOT**：`docs/adr/0000-engineering-charter.md`
- **Git 工作流**：`docs/adr/0007-git-workflow-policy.md`
- **注释体系**：`docs/adr/0005-comment-system.md`
- **日志体系**：`docs/adr/0006-logging-system-and-facade.md`
- **麻将规则引擎**：`docs/RULE-ENGINE.md`、`docs/ROOM-FSM.md`
- **Agent 治理体系**：`docs/adr/0042-agent-governance.md`、`docs/agent-governance/coverage-matrix.md`
- **CLI 纯中文 TUI 设计**：`docs/adr/0047-cli-pure-chinese-redesign.md`
- **根因修复策略**：`docs/adr/0043-root-cause-fix-policy.md`
- **Agent 双边治理**：`.claude/`、`.codex/skills/lsp-server-governance/`、`docs/agent-governance/coverage-matrix.md`
- **ADR 索引**：`docs/adr/README.md`
