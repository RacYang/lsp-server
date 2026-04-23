---
title: 房间亲和与单房间事件循环
status: accepted
date: 2026-04-22
---

# ADR-0011 房间亲和与单房间事件循环

## 状态

已采纳。

## 背景

[docs/ARCHITECTURE.md](../../ARCHITECTURE.md) 要求**每个房间单一事件循环 Goroutine**，多进程后仍需保证同房间请求串行、归属唯一。

## 决策

1. **归属权威**  
   - 每房间在 etcd 中记录 **当前负责节点**（与节点租约关联）；仅该 `room` 进程可执行该 `room_id` 的牌局状态迁移。  
   - `lobby` 在创建或首次绑定房间时 **Claim** 归属到某一 `room` 节点。  
   - `gate` 收到玩家请求时，先解析 `room_id`，按 [docs/CLUSTER.md](../../CLUSTER.md) 做**粘性路由**：同一房间在迁移完成前打向同一 `room` 节点。  

2. **单房间事件循环**  
   - 在 `room` 进程内，每个 `room_id` 一个 **mailbox**（有界 channel），仅该 goroutine 接触可变桌状态。  
   - 入站 gRPC/内部调用转化为 **事件** 入队，队列满时丢弃非关键或返回背压（实现：默认有界 1024，可配置，记 warn 日志）。  

3. **安全缩容**（与 CLUSTER 文档四步一致）  
   - 先标记不可调度、停止新房间、排空活跃房、再注销 etcd。  
   - Phase 2 在控制接口上可简化为**进程级** drain 信号（详细实现渐进补齐）。

## 后果

- 多房间并行通过「每房一协程」扩展；单房内仍严格串行。  
- 竞态时以 etcd 的 `Compare-And-Set` 或事务判定归属，避免双主。  

## 可验证项

- 单测/集成：同一 `room_id` 并发 `ApplyEvent` 仍得到与顺序执行一致的终态（可在 Phase 2 中随 room 包落地）。
