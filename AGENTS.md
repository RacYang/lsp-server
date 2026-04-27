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
9. Phase 4 交互房间 ADR：[ADR-0015](docs/adr/0015-phase4-interactive-room-loop.md)；当前主链路包含换三张/定缺确认、碰/杠抢答提示、重连快照等待态与 `HeartbeatReq` / `LeaveRoomReq` 基础处理。
