# 集群

## 现状

Phase 2 已从单进程演进为可拆分的 `gate` / `lobby` / `room` 三进程基线，同时保留 `cmd/all` 作为本地 in-process 冒烟入口。

## 节点

- `gate`：WebSocket 接入、会话注册、客户端帧编解码、房间事件转推。
- `lobby`：`cluster.v1.LobbyService`，负责建房、进房与房间元数据查询。
- `room`：`cluster.v1.RoomService`，负责单房事件循环、动作裁决、结算与事件流输出。

## 进程关系

1. 客户端连接到 `gate` 的 `/ws`。
2. `gate` 通过 `LobbyService.JoinRoom` 获取座位与房间归属。
3. `gate` 对目标房间建立 `RoomService.StreamEvents` 订阅。
4. 客户端 `ready` / `discard` / `pong` / `gang` / `hu` 都经 `gate` 映射到 `RoomService.ApplyEvent`。
5. `room` 产出的局内事件经 gRPC 流返回给 `gate`，再被映射为 `client.v1` 推送。

## 路由

房间亲和性基于房间 ID。权威归属由 etcd 控制面维护，`gate` 可使用 Redis 做只读路由缓存，但缓存 miss 或冲突时必须回源 etcd。

## 本地与测试

- `cmd/all`：单进程聚合模式，便于开发期快速冒烟。
- `cmd/gate`、`cmd/lobby`、`cmd/room`：独立进程模式，供 Phase 2 跨进程回放与后续部署使用。
- 当前仓库已包含跨进程四人交互式对局冒烟测试，验证 `gate -> lobby/room gRPC -> WebSocket 推送 -> 客户端动作回流` 主链路。

## Phase 3 断线重连（概要）

- `gate` 在启用 Redis 时签发 `session_token`，并在进房后绑定 `room_id` 与事件游标。
- 客户端重连时携带 `session_token` 登录；`gate` 先 `SnapshotRoom` 再按快照游标订阅 `StreamEvents` 以补帧。
- 详细契约见 [ADR-0014](adr/0014-reconnect-session-and-snapshot-cutover.md)。
- 跨进程回归：`internal/app` 中 `TestClusterReconnectLoginWithSessionToken`（`gate` 启用 miniredis 对应 Redis、`session_token` 重连、`SnapshotNotify` 后继续四人结算）。

### 房间进程冷启动与 etcd

`room` 在配置 `etcd.endpoints` 时会先向 etcd 注册 `room-local` 节点，再按 ownership 枚举归属自己的活跃房间；恢复时会结合 Redis `snapmeta`、PG `game_summaries` 与 `room_events` 推导座位与阶段后重建 actor，并在恢复完成前拒绝 `SnapshotRoom` / `StreamEvents` / `ApplyEvent`。

Phase 4 起，Redis `snapmeta` 追加 `round_json`，保存进行中牌局的最小可恢复事实：

- 当前轮到谁出牌。
- 当前是否处于自摸待决窗口。
- 最近一次弃牌与可中断的抢答窗口。
- 抢答窗口内的候选座位与可执行动作。
- 四家手牌、牌墙剩余顺序、定缺结果。
- 已累计番数与当前赢家座位。

恢复策略是：先用 `snapmeta` 直接重建最小运行态，再由 `room_events` / `StreamEvents` 继续补齐客户端可见历史。若 `round_json` 缺失，则不会再把房间错误恢复成不可交互的 `playing`，而是降级回可重新准备的阶段。`pong` / `gang` 抢答窗口会随候选座位与动作一起恢复，裁决仍由单房 actor 串行执行。

## 安全缩容

1. 将节点标记为不可调度。
2. 停止分配新房间。
3. 排空活跃房间。
4. 从服务发现注销。

## Phase 6 部署入口

- Kubernetes 基线清单位于 `deploy/k8s/base/`，三服务仍保持 `gate` / `lobby` / `room` 边界。
- 托管 Secret 示例位于 `deploy/k8s/overlays/example/`，仅保留 placeholder；真实凭据由 KMS 或托管 Secret Manager 投递。
- SLO recording 与 alerting rules 位于 `deploy/observability/`，对外承诺项与 ADR-0024 保持一致。
- PostgreSQL 恢复演练 runbook 位于 `deploy/ops/postgres-restore.md`，用于校验 `SnapshotRoom` 与 `StreamEvents` 的恢复可读性。
