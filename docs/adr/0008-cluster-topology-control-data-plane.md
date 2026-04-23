---
title: 集群拓扑与控制面与数据面划分
status: accepted
date: 2026-04-22
---

# ADR-0008 集群拓扑与控制面与数据面划分

## 状态

已采纳。

## 背景

Phase 2 将单体拆为 `gate`、`lobby`、`room` 多进程，并引入 **etcd** 与 **Redis**。若两类存储职责含混，易出现「双真相源」与路由脑裂，难以审计与回滚。

## 决策

1. **进程职责**（与 [docs/CLUSTER.md](../../CLUSTER.md) 一致）  
   - `gate`：WebSocket/帧、会话与扇出。  
   - `lobby`：登录、建房、进房、房间元数据与分配。  
   - `room`：单桌牌局事件循环、结算事件生成。  

2. **etcd（控制面）**  
   - 节点注册、租约心跳、**房间到节点归属**（强一致、低频写入）。  
   - 不承载会话热数据、不当作缓存。  

3. **Redis（数据面）**  
   - 在线会话、幂等键、重连快照元数据、**路由只读缓存**（可选）。  
   - **房间主归属以 etcd 为准**；Redis 中路由缓存 miss 时回源 etcd 再回填。  

4. **开发形态**  
   - `cmd/all` 可 in-process 聚合多角色以便本地冒烟；生产环境使用独立 `cmd/gate`、`cmd/lobby`、`cmd/room`。

## 后果

- 运维可依据 etcd 与 Redis 分工分别扩容与排障。  
- 所有实现须避免在 Redis 中写入「可充当唯一主归属」的 room→node 而绕过 etcd 校验。

## 本地与 CI 依赖

- 集成测试可依赖 `docker compose` 启动 etcd 与 Redis；单测中 etcd 子集可使用官方嵌入式用例。  
- `docker`/`docker compose` 不列入 `.build/config.yaml` 的 `verify-tools` 硬校验，避免与默认开发机环境不一致。
