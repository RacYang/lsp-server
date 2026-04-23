# 客户端协议（v1）

本文档描述 Phase 1 单体 MVP 的 WebSocket 二进制帧头与 Protobuf 载荷约定，对应 [ADR-0003](adr/0003-frame-protocol.md)。

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

## Phase 1 未覆盖项（TODO）

- 换三张、查花猪、查大叫、退税等川麻扩展结算。
- 断线重连与幂等会话恢复。
