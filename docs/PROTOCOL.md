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
| 31 | 开局手牌通知 | `initial_deal` | S→C |
| 32 | 房间列表请求 | `list_rooms_req` | C→S |
| 33 | 房间列表响应 | `list_rooms_resp` | S→C |
| 34 | 自动匹配请求 | `auto_match_req` | C→S |
| 35 | 自动匹配响应 | `auto_match_resp` | S→C |
| 36 | 创建房间请求 | `create_room_req` | C→S |
| 37 | 创建房间响应 | `create_room_resp` | S→C |
| 38 | 过请求 | `pass_req` | C→S |
| 39 | 过响应 | `pass_resp` | S→C |

## Phase 3 登录与重连（节选）

- `LoginRequest.session_token` 非空时表示尝试恢复；服务端校验 Redis 中的令牌摘要与会话记录。
- `LoginResponse.session_token` 为新签发或沿用（重连成功时与请求相同）的不透明令牌；`resumed` 表示是否恢复上下文；`resume_cursor` 为建议保存的事件游标。
- `session_token` 绑定用户会话，不绑定具体 `gate` 副本；同集群内任意 `gate` 都应能恢复大厅态或牌桌态。
- 重连成功后服务端可额外推送一帧 `msg_id=27` 的 `SnapshotNotify`，载荷为 `Envelope.snapshot`；其中 `your_hand_tiles` 仅包含当前连接所属座位的手牌，`discards_by_seat` 与 `melds_by_seat` 用于恢复弃牌堆与副露展示。`phase`、`acting_seats` 与 `last_step` 是局内恢复的权威切点，客户端应丢弃 `step <= last_step` 的陈旧推进事件。

## 业务错误码（ErrorCode 节选）

- `UNAUTHORIZED`：登录、会话或鉴权失败。
- `ROOM_NOT_FOUND`：目标房间不存在。
- `ROOM_FULL`：房间已满，无法加入。
- `INVALID_STATE`：请求与当前房间阶段、座位或等待态不匹配。
- `NOT_YOUR_TURN`：出牌或动作请求不属于当前可操作座位。
- `ROUTE_REDIRECT`：客户端应按 `RouteRedirectNotify.ws_url` 中的完整 WebSocket URL 切换连接；正常同集群恢复不应触发。
- `RATE_LIMITED`：请求过频，应退避重试。
- `RECONNECTING`：断线重连中（Phase 3 完整会话恢复前可作占位）。

## Phase 4 交互闭环

- `discard_req` 已打通到 `ws -> gate -> room.ApplyEvent -> room actor -> StreamEvents`，服务端进入真正的“等待摸牌/等待出牌”循环，而不是 `ready` 后自动整局回放。
- `Envelope.idempotency_key` 可由客户端为会改变房间状态的请求生成。WS 入口会对已知状态变更请求做进程内去重，未知 `msg_id` 不进入幂等缓存。
- `pong_req` / `gang_req` / `hu_req` / `pass_req` 都有显式响应帧；`hu_req` 支持自摸、点炮胡与抢杠胡窗口，`pass_req` 表示当前被询问玩家主动放弃本次抢答或自摸选择。
- 服务端只允许当前最高优先级候选响应抢答窗口。候选主动 `pass_req` 后，服务端移除该候选并接力下一 top candidate；若无人可抢，则关闭窗口并继续摸牌。每个接力候选拿到完整 `ClaimWindow`。
- 当某玩家摸牌后可自摸时，服务端先广播一条 `action.action = "tsumo_choice"` 的提示；客户端可发送 `hu_req` 胡牌，也可发送 `pass_req` 表示不胡。主动过会把摸到的牌加入手牌并进入 `discard` 等待态，让玩家自行选牌打出；服务端超时托管才会默认打出 `pendingDraw`。
- `SnapshotNotify` 现已追加 `acting_seat`、`acting_seats`、`phase`、`waiting_action`、`pending_tile`、`available_actions`、`claim_candidates` 与 `last_step`，用于重连后恢复当前等待态。`SnapshotNotify.state` 只表示房间 FSM（waiting/ready/playing/settling/closed），局内 UI 优先以 `phase` 为准；`waiting_action` 保留给旧客户端。
- `InitialDealNotify` 由 `room` 在完成开局发牌后按座位定向下发，每个连接只会收到自己座位的 13 张初始手牌；集群模式通过 `cluster.v1.RoomServiceStreamEventsResponse.target_seat` 传递定向语义，`-1` 表示广播。
- `DrawTileNotify` 通过每座位投影下发：摸牌本人收到具体 `tile`，其他座位收到空 `tile` 与相同 `seat_index`。隐私敏感局内事件不得全量明文广播。
- 服务端托管入口在当前等待态超时时可自动执行默认动作：抢答窗口选择最高优先级候选，出牌/自摸待决窗口默认打出确定性弃牌。玩家显式请求不会触发自动出牌；碰牌后必须等待该玩家下一次 `discard_req` 或托管超时。
- WS 入口有 token bucket 限流；room actor mailbox 也有有界队列。触发限流时响应 `ERROR_CODE_RATE_LIMITED` 或直接丢弃过频帧并计入指标。

## Phase 7 大厅列表与匹配

- `ListRoomsRequest` 返回 `RoomMeta` 列表，仅包含公开、未满且可加入的等待房间；私密房不出现在列表中。
- `JoinRoomResponse`、`AutoMatchResponse`、`CreateRoomResponse` 会返回 `rule_id` 与 `display_name`，客户端入桌后不再猜规则与房间名。
- `AutoMatchRequest.rule_id` 为空时使用默认规则。服务端会选择最早创建的可加入公开房；没有候选时创建一个公开房并直接返回 `room_id`、`seat_index` 与房间元数据。
- `CreateRoomRequest` 会创建房间并让创建者直接占座。`private=true` 时房间只能凭 `room_id` 手动加入。
- `RoomMeta.stage` 当前仅表达 lobby 视角的 `waiting`，不承载局内细分状态；重连与局内视图仍以 `SnapshotNotify` 为准。

## Phase 5 协议与观测补充

- `Envelope.idempotency_key` 会随所有请求载荷传递，WS 入口只对会改变房间状态的已知请求做进程内快速去重；跨进程幂等仍以 `RoomService.ApplyEvent.idempotency_key` 与 Redis 为准。
- `ActionNotify.action` 当前冻结为 `discard`、`pong`、`gang`、`hu`、`exchange_three`、`que_men`、`hu_choice`、`qiang_gang_choice`、`pong_choice`、`gang_choice`、`tsumo_choice`。新增跨端动作须同步协议文档与 ADR。
- `SnapshotNotify.claim_candidates` 与 `ActionNotify.action` 的 `hu_choice` / `qiang_gang_choice` / `pong_choice` / `gang_choice` 共同描述抢答窗口，重连客户端应优先以快照中的等待态恢复 UI。
- `SettlementNotify.per_winner_breakdown` 透传每个赢家的结构化分摊结果；包牌、退税、查花猪与查大叫等罚分仍通过结算字段表达，不依赖客户端重新推导。
- 未知 `msg_id` 不进入幂等缓存，也不会分配新的 `ErrorCode`；服务端以 `lsp_unknown_msg_total` 计数供观测。
- 幂等重放、限流与 actor 队列满分别进入 `lsp_idempotent_replay_total`、`lsp_rate_limited_total` 与 `lsp_actor_queue_depth`，客户端可见错误码仍只使用本页枚举。
