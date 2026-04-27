# 变更日志

## 未发布

- Phase 5.3：血战规则深化，新增听牌检测、查大叫精准化、score ledger 结算流水、自摸/点炮/抢杠胡分摊、杠分退税、包牌标记、`per_winner_breakdown` 客户端透传与将对/暗刻/暗杠等番种。
- BREAKING：Phase 5 重置 `proto-baseline`；`client.v1.Envelope` 追加 `idempotency_key`，`SnapshotNotify` / `SnapshotRoomResponse` 追加 `claim_candidates`，`cluster.v1.SettlementEvent` 补齐 `seat_scores` / `penalties` / `per_winner_breakdown`，跨进程结算不再丢失结构化罚分。
- Phase 5：`room` 交互引擎按职责拆分，统一 `sichuanxzdd` 包名，并为 Redis `snapmeta.round_json` 增加 `schema_version`，未知未来版本恢复时降级为重新准备而非阻断 room 启动。
- Phase 5：血战主链路支持胡后续行、点炮胡候选、抢杠胡窗口、胡/杠/碰优先级裁决、杠流水与更多番种上下文，删除 Phase 4 工程步数截断。
- Phase 5：引入可注入 `clock.Clock`、每房定时器、Hub 心跳超时、WS token bucket、actor 有界 mailbox、WS 幂等重放去重与 `ERROR_CODE_RATE_LIMITED` 指标。
- Phase 5：新增最小可观测指标集合（抢答窗口、自动托管、重连、actor 队列深度、存储耗时、限流/幂等/未知消息）与 ADR-0019；integration 目标覆盖重连、幂等、限流与 fakeClock 托管。
- Phase 4：`room` 改为 `exchange_three -> que_men -> start_game -> draw` 显式推进；`ExchangeThreeReq` / `QueMenReq`、`HeartbeatReq` / `LeaveRoomReq` 已接入服务端；碰/杠抢答改为多候选窗口，先下发 `pong_choice` / `gang_choice` 并按确定性优先级裁决，不再先广播下一家摸牌后回滚。
- 治理：`verify` 新增 `verify-test-integration` 钩子与 CI integration 作业；`cmd/all`、`cmd/gate` 入口补充测试，覆盖率门闸不再排除 `cmd/**` 与 `internal/app/**`。
- Phase 3：新增 ADR-0013/0014；`client.v1` 登录重连字段与 `SnapshotNotify`；`cluster.v1.RoomService.SnapshotRoom`；`room` 可选 PostgreSQL 事件日志与结算表、Redis 快照元数据、`StreamEvents` 游标重放；`gate` Redis 会话令牌与 `Resume` 主路径；Prometheus 指标与各进程可观测性 HTTP（`/healthz`、`/readyz`、`/metrics`、pprof）。
- Phase 2 完整集群基线：新增 `cmd/gate`、`cmd/lobby`、`cmd/room` 三进程入口，`gate` 可通过 `cluster.v1` gRPC 与 `lobby/room` 协作；`cmd/all` 明确降级为本地 in-process 冒烟入口。
- Phase 2：`room` gRPC 已对接真实房间 worker；四人 `ready` 后可跑完整四川血战自动回放，并通过跨进程 WebSocket 冒烟测试验证 `gate -> lobby/room` 主链路。
- Phase 2：引入 etcd 控制面与 Redis 数据面基础实现，补齐房间亲和、会话/幂等/路由缓存、`cluster.v1` 与 `client.v1` 协议扩展文档。
- Phase 1 单体 MVP：`cmd/all`、WebSocket 帧协议、房间与准备流、四客户端端到端结算冒烟；`proto-baseline` 标签与 `buf breaking` 基线；覆盖率门闸修复（模块路径归一化）；日志 message UTF-8 解析修正
- Phase 1：麻将算法层（万筒条）、牌墙、手牌、和牌判定、番种分解、可插拔规则与四川血战到底 MVP 规则包；JSON 夹具与架构分层修正。
- Phase 1 启动：SSOT `stage` 切换为 `alpha`，后续新增业务代码将按覆盖率门槛校验。
- 引导工程治理基线。
- 中文化与注释/日志治理：SSOT 三节、`verify-lang-*`、统一日志门面策略与负例。
