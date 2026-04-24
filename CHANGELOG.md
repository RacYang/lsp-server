# 变更日志

## 未发布

- Phase 3：新增 ADR-0013/0014；`client.v1` 登录重连字段与 `SnapshotNotify`；`cluster.v1.RoomService.SnapshotRoom`；`room` 可选 PostgreSQL 事件日志与结算表、Redis 快照元数据、`StreamEvents` 游标重放；`gate` Redis 会话令牌与 `Resume` 主路径；Prometheus 指标与各进程可观测性 HTTP（`/healthz`、`/readyz`、`/metrics`、pprof）。
- Phase 2 完整集群基线：新增 `cmd/gate`、`cmd/lobby`、`cmd/room` 三进程入口，`gate` 可通过 `cluster.v1` gRPC 与 `lobby/room` 协作；`cmd/all` 明确降级为本地 in-process 冒烟入口。
- Phase 2：`room` gRPC 已对接真实房间 worker；四人 `ready` 后可跑完整四川血战自动回放，并通过跨进程 WebSocket 冒烟测试验证 `gate -> lobby/room` 主链路。
- Phase 2：引入 etcd 控制面与 Redis 数据面基础实现，补齐房间亲和、会话/幂等/路由缓存、`cluster.v1` 与 `client.v1` 协议扩展文档。
- Phase 1 单体 MVP：`cmd/all`、WebSocket 帧协议、房间与准备流、四客户端端到端结算冒烟；`proto-baseline` 标签与 `buf breaking` 基线；覆盖率门闸修复（模块路径归一化）；日志 message UTF-8 解析修正
- Phase 1：麻将算法层（万筒条）、牌墙、手牌、和牌判定、番种分解、可插拔规则与四川血战到底 MVP 规则包；JSON 夹具与架构分层修正。
- Phase 1 启动：SSOT `stage` 切换为 `alpha`，后续新增业务代码将按覆盖率门槛校验。
- 引导工程治理基线。
- 中文化与注释/日志治理：SSOT 三节、`verify-lang-*`、统一日志门面策略与负例。
