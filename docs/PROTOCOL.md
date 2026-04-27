# 客户端协议（v1）

本文档描述单进程/集群基线下 WebSocket 二进制帧头与 Protobuf 载荷约定，对应 [ADR-0003](adr/0003-frame-protocol.md)。

## 集群模式补充

- 客户端始终只与 `gate` 建立 WebSocket 连接。
- `gate` 内部通过 `cluster.v1.LobbyService` / `cluster.v1.RoomService` 与 `lobby`、`room` 协作。
- `room` 输出的是 cluster 抽象事件，`gate` 再将其映射回本页定义的 `client.v1` 帧。
- 若未来发生房间迁移，`gate` 可使用 `route_redirect` 通知客户端重连到新的接入地址。

## 二进制帧头（9 字节，大端）

| 偏移 | 长度 | 字段       | 说明 |
|------|------|------------|------|
| 0    | 2    | `magic`    | 固定 `0x4C53`（ASCII `LS`） |
| 2    | 1    | `version`  | 当前为 `1` |
| 3    | 2    | `msg_id`   | 业务消息类型，见下表 |
| 5    | 4    | `payload_len` | Protobuf 字节长度 |

`msg_id` **仅**出现在帧头；载荷使用 `client.v1.Envelope`，其中 `oneof body` 与 `msg_id` 一一对应，避免双真相源。

## msg_id 与 Envelope.body 对照

| msg_id | 名称 | oneof 字段 | 方向 |
|--------|------|------------|------|
| 1 | 登录请求 | `login_req` | C→S |
| 2 | 登录响应 | `login_resp` | S→C |
| 3 | 进房请求 | `join_room_req` | C→S |
| 4 | 进房响应 | `join_room_resp` | S→C |
| 5 | 准备请求 | `ready_req` | C→S |
| 6 | 准备响应 | `ready_resp` | S→C |
| 7 | 开局通知 | `start_game` | S→C |
| 8 | 摸牌通知 | `draw_tile` | S→C |
| 9 | 出牌请求 | `discard_req` | C→S |
| 10 | 出牌响应 | `discard_resp` | S→C |
| 11 | 碰请求 | `pong_req` | C→S |
| 12 | 杠请求 | `gang_req` | C→S |
| 13 | 胡请求 | `hu_req` | C→S |
| 14 | 动作通知 | `action` | S→C |
| 15 | 结算通知 | `settlement` | S→C |
| 16 | 心跳请求 | `heartbeat_req` | C→S |
| 17 | 心跳响应 | `heartbeat_resp` | S→C |
| 18 | 离房请求 | `leave_room_req` | C→S |
| 19 | 离房响应 | `leave_room_resp` | S→C |
| 20 | 路由重定向 | `route_redirect` | S→C |
| 21 | 换三张请求 | `exchange_three_req` | C→S |
| 22 | 换三张响应 | `exchange_three_resp` | S→C |
| 23 | 换三张完成通知 | `exchange_three_done` | S→C |
| 24 | 定缺请求 | `que_men_req` | C→S |
| 25 | 定缺响应 | `que_men_resp` | S→C |
| 26 | 定缺完成通知 | `que_men_done` | S→C |
| 27 | 快照通知 | `snapshot` | S→C |
| 28 | 碰响应 | `pong_resp` | S→C |
| 29 | 杠响应 | `gang_resp` | S→C |
| 30 | 胡响应 | `hu_resp` | S→C |

## Phase 3 登录与重连（节选）

- `LoginRequest.session_token` 非空时表示尝试恢复；服务端校验 Redis 中的令牌摘要与会话记录。
- `LoginResponse.session_token` 为新签发或沿用（重连成功时与请求相同）的不透明令牌；`resumed` 表示是否恢复上下文；`resume_cursor` 为建议保存的事件游标。
- 重连成功后服务端可额外推送一帧 `msg_id=27` 的 `SnapshotNotify`，载荷为 `Envelope.snapshot`。

## 业务错误码（ErrorCode 节选）

- `ROUTE_REDIRECT`：客户端应按 `RouteRedirectNotify` 切换连接。
- `RATE_LIMITED`：请求过频，应退避重试。
- `RECONNECTING`：断线重连中（Phase 3 完整会话恢复前可作占位）。

## Phase 4 交互闭环

- `discard_req` 已打通到 `ws -> gate -> room.ApplyEvent -> room actor -> StreamEvents`，服务端进入真正的“等待摸牌/等待出牌”循环，而不是 `ready` 后自动整局回放。
- `pong_req` / `gang_req` / `hu_req` 都有显式响应帧；当前其中 `hu_req` 用于自摸待决窗口，`pong` / `gang` 支持对最近一次弃牌打开多候选抢答窗口。服务端会向可抢座位分别下发 `pong_choice` / `gang_choice`，并按“杠优先于碰、同优先级按出牌座位下家顺序”的规则裁决。
- 当某玩家摸牌后可自摸时，服务端先广播一条 `action.action = "tsumo_choice"` 的提示；客户端随后可发送 `hu_req`，也可直接对该摸到的牌发送 `discard_req` 继续轮转。
- `SnapshotNotify` 现已追加 `acting_seat`、`waiting_action`、`pending_tile`、`available_actions`，用于重连后恢复当前等待态。
- 服务端托管入口在当前等待态超时时可自动执行默认动作：抢答窗口选择最高优先级候选，出牌/自摸待决窗口默认打出确定性弃牌。
