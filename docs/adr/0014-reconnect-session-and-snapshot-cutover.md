---
title: 断线重连、会话校验与快照回放切点
status: accepted
date: 2026-04-22
---

# ADR-0014 断线重连、会话校验与快照回放切点

## 状态

已采纳。

## 当前实现状态

ADR-0040/RuleState opaque 化后，`round_json` 的恢复边界从“某个血战快照字段集合”升级为通用局面事实、`rule_id`、opaque `rule_state`、`win_events` 与 `score_events`。当前实现已硬切：低版本、缺版本或高版本快照都不可恢复，由上层降级重新准备。

## 背景

客户端 WebSocket 易断；`gate` 需在重连后恢复房间上下文。若 `session_token` 仅可解析而不可校验，任意客户端可冒充他人会话；若快照与 `StreamEvents` 回放边界不清，会出现重复或漏事件。

## 决策

### 1. 会话令牌与 Redis 会话记录

- `client.v1.LoginRequest` 可携带 `session_token`（空表示新登录）。  
- 新登录：`gate` 生成随机不透明 `session_token`，计算 `token_hash`（如对 token 做 SHA-256 十六进制摘要），与 `session_ver`（单调版本，初始为 1）一并写入 `lsp:session:{user_id}`（JSON 见 `internal/store/redis.SessionRecord`）。  
- 响应中返回明文 `session_token` 与 `user_id`；**Redis 仅存 hash，不存明文 token**。  
- 重连：客户端提交 `session_token`；服务端根据 `user_id`（见下）取出 `SessionRecord`，比对 `token_hash` 与 `session_ver`；不匹配则拒绝恢复（`ERROR_CODE_UNAUTHORIZED` 或走新登录）。  
- **user_id 与 token 的绑定**：除 `lsp:session:{user_id}` 存完整 `SessionRecord` 外，另设辅助键 `lsp:session:lookup:{token_hash}` → `user_id`（短 TTL 与会话一致），便于重连仅凭不透明 `session_token` 反查用户；校验流程为：对明文 token 求 hash → `GET lookup` 得 `user_id` → `GET session:{user_id}` 比对 `TokenHash` 一致后视为有效。
- `session_token` 绑定用户会话，不绑定具体 `gate` 副本。同集群内任意 `gate` 都可校验该 token 并恢复大厅态或房间态；`SessionRecord` 中历史保留的 `gate_node_id` / `advertise_addr` 不得作为同集群恢复的拒绝或重定向依据。

### 2. 快照与回放切点（避免重复/漏帧）

- `cluster.v1.RoomService.SnapshotRoom` 返回**快照游标** `snapshot_cursor`（格式同 [ADR-0013](0013-persistence-model-and-event-cursor.md)）。  
- `gate` 恢复流程：**先** `SnapshotRoom`，**再** `StreamEvents(since_cursor = snapshot_cursor)`。  
- `room` 侧 `StreamEvents`：先按 `since_cursor` 从 PG `ListEventsSince` 重放历史事件，再注册 live 订阅并按 cursor 去重接续内存尾流；保证 `snapshot_cursor` 之后的事件在 replay/live cutover 中不重复、不漏发。  
- 若客户端本地 `last_client_cursor` 已晚于 `snapshot_cursor`，`gate` 可对下游推送按 `req_id`/cursor 去重。
- `SnapshotRoomRequest.user_id` 用于生成当前连接的私有恢复视图；`SnapshotNotify.your_hand_tiles` 仅填入该用户所在座位的手牌，`discards_by_seat` 与 `melds_by_seat` 则返回四家可见历史，供终端或图形客户端重建牌桌。

### 3. room 进程重启与最小牌局恢复

- 归属以 etcd 为准（[ADR-0008](0008-cluster-topology-control-data-plane.md)、[ADR-0011](0011-room-affinity-routing.md)）。  
- `room` 进程启动后，对当前节点 claim 的活跃 `room_id`：读 Redis `snapmeta`、PG `game_summaries` 与 `room_events` 推导到一致 `seq` 的最小一致状态并重建单房 actor；恢复完成前对该房拒绝 `SnapshotRoom` / `StreamEvents` / `ApplyEvent` 或返回 `ERROR_CODE_RECONNECTING`。
- `snapmeta` 中的 `round_json` 保存进行中局的最小权威事实：轮到谁、是否处于自摸待决、碰/杠抢答候选窗口、四家手牌、剩余牌墙、`rule_id`、opaque `rule_state`、`win_events` 与 `score_events`。定缺、换三张提交态等规则私有事实只通过当前 `rule_state` 保存和投影。
- 恢复后保证继续处理 `discard_req` / `hu_req` / 合法的 `pong_req` / `gang_req`，并通过 `StreamEvents(since_cursor=snapshot_cursor)` 让客户端补齐可见历史。
- 碰/杠抢答窗口按候选座位与候选动作恢复；更复杂的弃牌后胡牌优先级、过手限制等规则仍通过后续 ADR 收敛。

### 4. 客户端可见结果

- `resumed=false` 且 `room_id` 为空：表示 token 有效但当前用户处于大厅态，仅返回 `LoginResponse`，不下发快照。  
- `resumed=true`：下发快照通知（含当前玩家手牌、弃牌堆与副露）+ 后续事件流。  
- `resumed=false` 且局已结：可下发结算摘要（以 PG 为准）。  
- 无法恢复：`ERROR_CODE_RECONNECTING`，或在跨入口 / 跨区域迁移时下发 `RouteRedirectNotify`（与 [client.v1 ErrorCode](../../api/proto/client/v1/messages.proto) 一致）。`RouteRedirectNotify.ws_url` 必须是客户端可直接拨号的完整 `ws://` / `wss://` URL；正常同集群 `gate` 副本恢复不应下发。

### 5. 断线托管边界

- WebSocket 断开只标记座位离线，不立即清理 lobby 的 `userIndex`；用户仍可凭 `session_token` 在 `runtime.room.surrender_after_offline` 窗口内回到同一房间。
- 若超时前用户重新注册到原房间连接，离线托管任务必须取消，不得把短线重连玩家标记为 surrender。
- 若超过 `surrender_after_offline` 仍未重连，gate 向 room 投递与主动离房相同的 surrender 路径；playing 中保留原 `user_id` 以便结算归属与后续恢复语义一致。

## 后果

- `gate` 必须依赖 Redis 与会话装配；单进程 `cmd/all` 须注入相同抽象以保持行为一致。  
- Proto 仅追加字段与 RPC，不移动 `proto-baseline` 标签（见 [ADR-0012](0012-proto-baseline-and-versioning.md)）。
