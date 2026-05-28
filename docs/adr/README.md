# ADR 索引

本文档是架构决策记录的导航页。CLAUDE.md 只保留执行纪律；完整阶段脉络以本索引为准。

## Phase 0：治理基线

- [ADR-0000 工程宪章](0000-engineering-charter.md)：已采纳；定义 SSOT、派生、负例与 verify 基线。
- [ADR-0001 自研路线与拒绝框架](0001-self-built-vs-framework.md)：已采纳；拒绝游戏服务器框架，保留聚焦单点库。
- [ADR-0002 可插拔规则接口](0002-pluggable-rule-interface.md)：已采纳；定义麻将规则接口与变体边界。
- [ADR-0003 帧协议](0003-frame-protocol.md)：已采纳；约定 WebSocket 帧与 Protobuf 载荷关系。
- [ADR-0004 语言与书写策略](0004-language-and-writing-policy.md)：已采纳；统一文档、注释与日志中文化口径。
- [ADR-0005 注释体系](0005-comment-system.md)：已采纳；规定包、类型、导出函数与关键分支注释。
- [ADR-0006 日志体系与门面](0006-logging-system-and-facade.md)：已采纳；统一 logx 门面、字段与直调限制。
- [ADR-0007 Git 工作流策略](0007-git-workflow-policy.md)：已采纳；规定分支、提交、tag、hook 与 CI 对齐。

## Phase 2：集群基线

- [ADR-0008 集群拓扑与控制面/数据面](0008-cluster-topology-control-data-plane.md)：已采纳；划分 gate、lobby、room 与 etcd/Redis 职责。
- [ADR-0009 服务间通信](0009-inter-service-communication.md)：已采纳；约定 gRPC、元数据与服务边界。
- [ADR-0010 Redis 键规范](0010-redis-key-layout.md)：已采纳；定义 Redis key 前缀、用途与 TTL 语义。
- [ADR-0011 房间亲和与事件循环](0011-room-affinity-routing.md)：已采纳；确定房间 worker、路由缓存与亲和策略。
- [ADR-0012 Proto 基线与版本策略](0012-proto-baseline-and-versioning.md)：已采纳；约定单模块 proto-baseline 与兼容演进。

## Phase 3：持久化与重连

- [ADR-0013 持久化模型与事件游标](0013-persistence-model-and-event-cursor.md)：已采纳；定义事件日志、快照摘要与游标回放。
- [ADR-0014 断线重连与快照切点](0014-reconnect-session-and-snapshot-cutover.md)：已采纳；定义会话校验、快照与事件续传。

## Phase 4：交互房间

- [ADR-0015 交互式房间事件循环](0015-interactive-room-loop.md)：已采纳；落地换三张、定缺、抢答与基础心跳离房。

## Phase 5：规则与韧性

- [ADR-0016 协议基线重置](0016-proto-baseline-reset.md)：已采纳；重置 Phase 5 proto-baseline 并扩展快照版本。
- [ADR-0017 房间引擎与结算边界](0017-room-engine-and-settlement-boundary.md)：已采纳；拆分 room 引擎职责与结算持久化边界。
- [ADR-0018 定时器与心跳时钟](0018-room-timer-and-heartbeat-clock.md)：已采纳；引入可注入时钟、托管超时与 Hub 心跳。
- [ADR-0019 可观测指标](0019-observability-metrics.md)：已采纳；定义最小 Prometheus 指标集合。
- [ADR-0020 血战规则深化](0020-rules-deepening.md)：已采纳；补齐听牌、查大叫、杠分、包牌与得分明细。
- [ADR-0021 庄家与高阶番种](0021-dealer-and-advanced-fans.md)：已采纳；引入庄家上下文、天胡、地胡、龙七对与十八罗汉。
- [ADR-0022 运行时参数与存储弹性](0022-runtime-knobs-and-storage-resilience.md)：已采纳；定义 runtime knob 与 Redis/PostgreSQL 重试边界。

## Phase 6：生产交付

- [ADR-0023 范围与路线](0023-scope-and-roadmap.md)：已采纳；限定 Phase 6 部署、SLO、压测与容量议题。
- [ADR-0024 部署与 SLO](0024-deployment-and-slo.md)：已采纳；定义镜像、Kubernetes 清单、灰度回滚与 SLO 子集。
- [ADR-0025 压测与容量](0025-load-and-capacity.md)：已采纳；定义 loadgen 场景、容量输出与基线回写。

## Phase 6+：运维深化

- [ADR-0026 PostgreSQL 备份与恢复](0026-postgres-backup-and-restore.md)：已采纳；定义备份、恢复演练与 RPO/RTO 记录。
- [ADR-0027 密钥与凭据管理](0027-secret-and-credential-management.md)：已采纳；定义 Secret 不入仓、轮换与 overlay 示例。
- [ADR-0028 跨地域多活](0028-multi-region-topology.md)：草案；评估多地域拓扑、成本与暂缓边界。
- [ADR-0029 签名提交升级评估](0029-signed-commit-required.md)：已采纳；定义签名提交试运行与升级路径。
- [ADR-0030 单机 Docker Compose 部署形态](0030-single-host-compose-deploy.md)：已采纳；与 ADR-0024 并存，限定单机 / 小规模 / 单租户场景。
- [ADR-0031 大厅列表与自动匹配协议](0031-lobby-list-and-matchmaking.md)：已采纳；定义客户端可见的房间列表、自动匹配与创建房间契约。
- [ADR-0032 lsp-cli 二进制分发](0032-cli-binary-distribution.md)：**已退役**；cmd/cli 删除后编号保留以维持序列连续性。
- [ADR-0033 日志采样与 PII 脱敏策略](0033-logging-sampling-and-pii-redaction.md)：已采纳；定义默认关闭采样、Error 永不采样与键名层 PII 边界。
- [ADR-0034 日志动态级别管控](0034-logging-dynamic-level-control.md)：已采纳；限定 Phase 1 仅提供 AtomicLevel Go API，不开放未鉴权 HTTP 写端点。
- [ADR-0035 OTel Logs Bridge 评估](0035-otel-logs-bridge.md)：已采纳；默认关闭 OTel 日志桥接，以 build tag 预留 trace/span 注入路径。
- [ADR-0036 静态分析与 nolint 治理策略](0036-static-analysis-and-lint-policy.md)：已采纳；定义 golangci-lint SSOT、全局排除与行级 nolint 豁免边界。
- [ADR-0037 Bot Supervisor 与 AddBot RPC](0037-bot-supervisor-and-add-bot-rpc.md)：已采纳；定义一键补位机器人、AddBot RPC 与 supervisor 后续边界。
- [ADR-0038 CLI 对称 TUI 布局](0038-cli-symmetric-tui-layout.md)：**已退役**；cmd/cli 删除后编号保留以维持序列连续性。
- [ADR-0039 四川血战权威局内契约](0039-sichuan-xuezhandaodi-authoritative-round-contract.md)：已采纳；定义玩家/托管命令分流、权威阶段、局共识、每座位投影与客户端 req_id 关联。
- [ADR-0040 组合式麻将规则能力](0040-composable-mahjong-rule-capabilities.md)：已采纳；定义换三张、定缺、吃碰杠胡、轮转、计分与结算的规则能力组合边界。
- [ADR-0041 麻将规则 ID 命名规范](0041-mahjong-rule-id-naming.md)：已采纳；统一 `<region>_<variant>_<option>` 全拼音规则 ID，并禁止缩写与混合语言。
- [ADR-0042 Agent 治理体系职责边界](0042-agent-governance.md)：已采纳；定义 AGENTS、ADR、rules、skills、templates 与 harness 的职责边界。
- [ADR-0043 根因优先的问题处理策略](0043-root-cause-fix-policy.md)：已采纳；定义查问题与修问题必须先解释根因、拒绝补丁式修复。
- [ADR-0044 房间状态与客户端契约](0044-room-state-and-client-contract.md)：**已退役**；前端删除后服务端侧职责由 ADR-0039/0040/0045 管辖，编号保留以维持序列连续性。
- [ADR-0045 房间 deadline 单一所有权与 PhaseUpdate/PhaseToken 契约](0045-phase-deadline-single-owner-and-phase-token.md)：已采纳；deadline 派生自 phaseStart+phaseReason，engine 经 enterPhase 写入，scheduler 退化为定时器对齐，协议层强制嵌入 PhaseUpdate 并以 PhaseToken 校验请求。
- [ADR-0046 治理体系迁移至 .claude/ 唯一事实源](0046-unified-claude-code-governance.md)：已采纳；以 `.claude/` 为唯一 SSOT，统一路径、术语、格式与 harness，完成 Cursor→Claude Code 迁移闭环。
- [ADR-0047 CLI 纯中文 TUI 重设计](0047-cli-pure-chinese-redesign.md)：**已退役**；cmd/cli 删除后编号保留以维持序列连续性。
- [ADR-0048 Agent 优先的治理文档格式](0048-agent-first-governance-format.md)：已采纳；治理资产以 agent 解析效率为优化目标，禁止散文，固定结构。
- [ADR-0049 零债务重构策略](0049-zero-debt-refactor-strategy.md)：已采纳；所有变更前必须全系统梳理并找到最优落点，禁止补丁式修复。
