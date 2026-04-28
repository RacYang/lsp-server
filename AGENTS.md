# AGENTS

请遵守本仓库的治理流水线：

1. 治理源变更请编辑 `.build/config.yaml`。
2. 派生产物请运行 `make generate`。
3. 在完成实质性工作前运行 `make verify`。
4. 保持麻将逻辑与传输层、存储层代码隔离。
5. 文档、注释与日志 message 以中文为主（见 ADR-0004～0006 与 `make verify-lang`）。
6. Git 工作流见 [ADR-0007](docs/adr/0007-git-workflow-policy.md)：在 `main` 外使用 `feat/`、`fix/` 等 topic 分支命名；不要依赖 `--no-verify` 跳过 hook，除非人类维护者明确授权。
7. Phase 2 集群相关 ADR：[ADR-0008](docs/adr/0008-cluster-topology-control-data-plane.md) 集群拓扑与控制面/数据面、[ADR-0009](docs/adr/0009-inter-service-communication.md) 服务间通信、[ADR-0010](docs/adr/0010-redis-key-layout.md) Redis 键规范、[ADR-0011](docs/adr/0011-room-affinity-routing.md) 房间亲和与事件循环、[ADR-0012](docs/adr/0012-proto-baseline-and-versioning.md) Proto 基线与版本策略。
8. Phase 3 持久化与重连 ADR：[ADR-0013](docs/adr/0013-persistence-model-and-event-cursor.md) 持久化模型与事件游标、[ADR-0014](docs/adr/0014-reconnect-session-and-snapshot-cutover.md) 断线重连与会话校验、快照回放切点。
9. Phase 4 交互房间 ADR：[ADR-0015](docs/adr/0015-interactive-room-loop.md)；当前主链路包含换三张/定缺确认、碰/杠抢答提示、重连快照等待态与 `HeartbeatReq` / `LeaveRoomReq` 基础处理。
10. Phase 5 规则与韧性 ADR：[ADR-0016](docs/adr/0016-proto-baseline-reset.md) 协议基线重置、[ADR-0017](docs/adr/0017-room-engine-and-settlement-boundary.md) 房间引擎与结算边界、[ADR-0018](docs/adr/0018-room-timer-and-heartbeat-clock.md) 定时器与心跳、[ADR-0019](docs/adr/0019-observability-metrics.md) 可观测指标、[ADR-0020](docs/adr/0020-rules-deepening.md) 血战规则深化、[ADR-0021](docs/adr/0021-dealer-and-advanced-fans.md) 庄家与高阶番种、[ADR-0022](docs/adr/0022-runtime-knobs-and-storage-resilience.md) 运行时参数与存储弹性。
11. Phase 6 生产交付 ADR：[ADR-0023](docs/adr/0023-scope-and-roadmap.md) 范围与路线、[ADR-0024](docs/adr/0024-deployment-and-slo.md) 部署与 SLO、[ADR-0025](docs/adr/0025-load-and-capacity.md) 压测与容量。
12. Phase 6+ 运维深化 ADR：[ADR-0026](docs/adr/0026-postgres-backup-and-restore.md) PostgreSQL 备份与恢复、[ADR-0027](docs/adr/0027-secret-and-credential-management.md) 密钥与凭据管理、[ADR-0028](docs/adr/0028-multi-region-topology.md) 跨地域多活草案、[ADR-0029](docs/adr/0029-signed-commit-required.md) 签名提交升级评估。
13. 除非人类维护者明确要求，本仓库未来规划不得引入第二套规则集或相关 ADR。
