---
title: Redis 键与失效语义
status: accepted
date: 2026-04-22
---

# ADR-0010 Redis 键与失效语义

## 状态

已采纳。

## 背景

Redis 将承载会话、幂等与快照元数据；若无统一前缀与 TTL，键冲突与泄露难以治理。

## 决策

1. **命名空间前缀**（固定 `lsp:` 前缀，后续字段小写与冒号分层）  
   - `lsp:session:{user_id}` — 用户当前绑定的 `gate` 与逻辑会话信息（如 JSON）。  
   - `lsp:session:lookup:{token_hash}` — 由会话令牌摘要反查 `user_id`（Phase 3 重连；值仅存 `user_id` 字符串）。  
   - `lsp:idem:{scope}:{idempotency_key}` — 幂等回复缓存（`scope` 为业务名，如 `join_room`）。  
   - `lsp:room:snapmeta:{room_id}` — 重连/快照元数据摘要（可含版本号、时间戳，具体序列化在实现中定）。  
   - `lsp:route:room:{room_id}` — **从 etcd 回源后的** 房间到节点只读缓存（非权威）。  

2. **TTL 原则**  
   - 幂等键：与业务幂等窗口一致（实现默认建议 5～30 分钟，可配置）。  
   - 会话与路由缓存：短 TTL（如 30～300 秒），以 etcd 为权威。  
   - 快照元数据：与房间生命周期或断线重连策略一致（Phase 2 为易失、与房间关闭时清理为主）。  

3. **与 etcd 关系**（见 [ADR-0008](0008-cluster-topology-control-data-plane.md)）  
   - 任一对 `lsp:route:room:*` 的写入，必须在逻辑上**可由 etcd 的 ownership 重算**；不得单独依赖 Redis 决定请求路由。

## 后果

- 实现层须在 `internal/store/redis` 集中封装，避免在业务中散落 `SET/GET` 键名。  
- 硬约束的键名/前缀变更需经 ADR 修订，并在 CI 中可用脚本抽查（随 Phase 2 实现逐步落地）。

## 与「硬约束负例」

- 在引入 store 的 enforcer 时，可添加负例：业务包直接拼 `lsp:` 键字符串（应被禁止，指向 store 包）。
