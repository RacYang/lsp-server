---
title: 断线重连、会话校验与快照回放切点
status: accepted
date: 2026-04-22
---

# ADR-0014 断线重连、会话校验与快照回放切点

## 状态

已采纳。

## 背景

客户端 WebSocket 易断；`gate` 需在重连后恢复房间上下文。若 `session_token` 仅可解析而不可校验，任意客户端可冒充他人会话；若快照与 `StreamEvents` 回放边界不清，会出现重复或漏事件。

## 决策

### 1. 会话令牌与 Redis 会话记录

- `client.v1.LoginRequest` 可携带 `session_token`（空表示新登录）。  
- 新登录：`gate` 生成随机不透明 `session_token`，计算 `token_hash`（如对 token 做 SHA-256 十六进制摘要），与 `session_ver`（单调版本，初始为 1）一并写入 `lsp:session:{user_id}`（JSON 见 `internal/store/redis.SessionRecord`）。  
- 响应中返回明文 `session_token` 与 `user_id`；**Redis 仅存 hash，不存明文 token**。  
- 重连：客户端提交 `session_token`；服务端根据 `user_id`（见下）取出 `SessionRecord`，比对 `token_hash` 与 `session_ver`；不匹配则拒绝恢复（`ERROR_CODE_UNAUTHORIZED` 或走新登录）。  
- **user_id 与 token 的绑定**：除 `lsp:session:{user_id}` 存完整 `SessionRecord` 外，另设辅助键 `lsp:session:lookup:{token_hash}` → `user_id`（短 TTL 与会话一致），便于重连仅凭不透明 `session_token` 反查用户；校验流程为：对明文 token 求 hash → `GET lookup` 得 `user_id` → `GET session:{user_id}` 比对 `TokenHash` 一致后视为有效。

### 2. 快照与回放切点（避免重复/漏帧）

- `cluster.v1.RoomService.SnapshotRoom` 返回**快照游标** `snapshot_cursor`（格式同 [ADR-0013](0013-persistence-model-and-event-cursor.md)）。  
- `gate` 恢复流程：**先** `SnapshotRoom`，**再** `StreamEvents(since_cursor = snapshot_cursor)`。  
- `room` 侧 `StreamEvents`：先按 `since_cursor` 从 PG `ListEventsSince` 重放历史事件，再注册 live 订阅并按 cursor 去重接续内存尾流；保证 `snapshot_cursor` 之后的事件在 replay/live cutover 中不重复、不漏发。  
- 若客户端本地 `last_client_cursor` 已晚于 `snapshot_cursor`，`gate` 可对下游推送按 `req_id`/cursor 去重。

### 3. room 进程重启

- 归属以 etcd 为准（[ADR-0008](0008-cluster-topology-control-data-plane.md)、[ADR-0011](0011-room-affinity-routing.md)）。  
- `room` 进程启动后，对当前节点 claim 的活跃 `room_id`：读 Redis `snapmeta`、PG `game_summaries` 与 `room_events` 推导到一致 `seq` 的**可恢复简化局况**并重建单房 actor；恢复完成前对该房拒绝 `SnapshotRoom` / `StreamEvents` / `ApplyEvent` 或返回 `ERROR_CODE_RECONNECTING`。当前基线不尝试完整逐手牌复原，而是恢复到可继续重连/继续准备/继续补帧的最小一致状态。

### 4. 客户端可见结果

- `resumed=true`：下发快照通知 + 后续事件流。  
- `resumed=false` 且局已结：可下发结算摘要（以 PG 为准）。  
- 无法恢复：`ERROR_CODE_RECONNECTING` 或 `RouteRedirectNotify`（与 [client.v1 ErrorCode](../../api/proto/client/v1/messages.proto) 一致）。

## 后果

- `gate` 必须依赖 Redis 与会话装配；单进程 `cmd/all` 须注入相同抽象以保持行为一致。  
- Proto 仅追加字段与 RPC，不移动 `proto-baseline` 标签（见 [ADR-0012](0012-proto-baseline-and-versioning.md)）。
