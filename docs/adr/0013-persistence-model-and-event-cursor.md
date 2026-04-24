---
title: 持久化模型与事件游标
status: accepted
date: 2026-04-22
---

# ADR-0013 持久化模型与事件游标

## 状态

已采纳。

## 背景

Phase 2 已将 `gate` / `lobby` / `room` 拆分，Redis 与 etcd 承担会话与路由；Phase 3 需要**可恢复事实**：断线重连、进程重启后仍能一致回放局内事件。若无统一游标与 append-only 事件日志，易出现双写、漏帧或重复帧。

## 决策

1. **PostgreSQL（权威事件日志与结算）**  
   - `room_events`：按 `room_id` + `seq` 单调递增的 append-only 事件表；`payload` 存序列化后的业务载荷（与 `cluster.v1` / `client.v1` 映射一致）。  
   - `game_summaries`：对局摘要（创建时间、规则、玩家列表、结束时间）。  
   - `settlements`：结算历史（赢家、番数、可读摘要等）。

2. **Redis（易失快照元数据）**  
   - 键名遵循 [ADR-0010](0010-redis-key-layout.md)：`lsp:room:snapmeta:{room_id}`。  
   - 值至少包含：`seq`（与 PG 对齐）、`player_ids`、`que_suits`（若适用）、`state`（FSM 字符串）、`updated_at`。  
   - 会话键 `lsp:session:{user_id}` 见 [ADR-0014](0014-reconnect-session-and-snapshot-cutover.md)。

3. **游标格式**  
   - 稳定游标：`cursor = "{room_id}:{seq}"`，其中 `seq` 为 `room_events.seq`（每房从 1 递增）。  
   - `cluster.v1.RoomServiceStreamEventsResponse.cursor` 与客户端 `Envelope.req_id` 承载该字符串，便于 gate 与客户端去重。

4. **写入路径**  
   - 房间侧在**统一**「产出通知并对外发布」路径中：先 `AppendEvent` 到 PG（分配 `seq`），再更新 Redis `snapmeta`，再推流；不得仅在单入口（如仅 `Ready`）写日志，以免未来交互事件遗漏。

## 后果

- 迁移与运维需引入 PostgreSQL；CI 默认以 mock 单测为主，集成测试可选带 tag。  
- 游标格式变更视为协议级变更，须同步修订 [docs/PROTOCOL.md](../../PROTOCOL.md) 与 [docs/STORAGE.md](../../STORAGE.md)。

## 与相关 ADR

- [ADR-0008](0008-cluster-topology-control-data-plane.md) 控制面/数据面划分。  
- [ADR-0010](0010-redis-key-layout.md) Redis 键布局。  
- [ADR-0014](0014-reconnect-session-and-snapshot-cutover.md) 重连与会话、快照切点。
