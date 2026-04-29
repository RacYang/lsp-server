---
title: 大厅列表与自动匹配协议
status: accepted
date: 2026-04-29
---

# ADR-0031 大厅列表与自动匹配协议

## 状态

已采纳。本 ADR 扩展 [ADR-0015](0015-interactive-room-loop.md) 的客户端交互范围，把「进房前」的大厅列表、自动匹配与显式创建纳入稳定协议。

## 背景

`lsp-cli` 最初依赖命令行参数或命令栏输入 `join <room_id>` 直接进入指定房间。该模式适合调试，但不满足玩家客户端的基本大厅体验：

1. 玩家无法知道当前有哪些公开房间可加入。
2. 玩家无法请求系统自动选择空位房间。
3. 玩家无法显式创建公开或私密房间。

现有 `LobbyService` 只有 `CreateRoom`、`JoinRoom`、`GetRoom` 三个集群内 RPC，且 `JoinRoomRequest` 必须携带 `room_id`。因此需要把大厅能力补齐到客户端 WebSocket 协议和 gate/lobby gRPC 契约中。

## 决策

### 1. 客户端可见协议

在 `client.v1` 中新增大厅摘要与三组请求/响应：

- `RoomMeta`：公开房间摘要，包含 `room_id`、`rule_id`、`display_name`、`seat_count`、`max_seats`、`created_at_ms` 与 `stage`。
- `ListRoomsRequest/ListRoomsResponse`：列出可加入的公开等待房。
- `AutoMatchRequest/AutoMatchResponse`：按规则选择最早创建的可加入房；没有候选时创建公开房。
- `CreateRoomRequest/CreateRoomResponse`：创建房间并让创建者直接入座；`private=true` 的房间不出现在列表中，只能凭 `room_id` 加入。

新增 `msg_id` 仅追加，不复用旧值：`32..37` 分别对应 list、auto-match、create 的请求与响应。`Envelope.body` 字段从 `34` 起追加，避开既有 `idempotency_key=32` 与 `initial_deal=33`。

### 2. Lobby 侧语义

大厅维护最小房间元数据：

- `rule_id` 为空时归一为默认 `sichuan_xzdd`。
- `display_name` 为空时使用 `room_id`。
- `max_seats` 固定为 4。
- `stage` 暂为 `waiting`，本 ADR 不引入局中房间看板。
- 公开列表只返回 `!private && seat_count < max_seats` 的房间。

房间 ID 由 lobby 生成 6 位短码，字符集排除易混淆字符。若生成结果碰撞，最多重试有限次数，避免无界循环。

### 3. 自动匹配

自动匹配按以下顺序执行：

1. 过滤规则匹配、公开、未满、等待中的房间。
2. 按 `created_at_ms` 升序、`room_id` 字典序稳定排序。
3. 加入第一个候选。
4. 无候选时创建一个公开房并返回创建者座位。

该语义是「快速加入空房」而非排队撮合；不提供跨规则、段位、延迟或地区等复杂匹配条件。

### 4. 持久化边界

大厅元数据与现有座位映射保持同一持久化级别：进程内内存。lobby 重启后列表会清空，已有房间的局内事件仍由 room/PostgreSQL 负责。后续若需要可恢复大厅列表，应另立 ADR 讨论 lobby 元数据持久化。

## 后果

- `lsp-cli` 可以在登录后展示大厅页，并支持刷新列表、自动匹配、创建公开/私密房。
- 旧客户端仍可继续使用 `JoinRoomRequest.room_id` 手动进房。
- gate 需要新增三个 WS handler，并把请求桥接到本地或远程 lobby。
- lobby gRPC 契约向后兼容追加，不破坏既有 `CreateRoom/JoinRoom/GetRoom` 调用。
- 大厅列表不是强一致看板；它只表达「当前 lobby 进程知道的可加入公开房间」。
