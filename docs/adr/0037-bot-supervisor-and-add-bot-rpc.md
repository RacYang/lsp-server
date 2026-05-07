---
title: Bot Supervisor 与 AddBot RPC
status: accepted
date: 2026-05-07
---

# ADR-0037：Bot Supervisor 与 AddBot RPC

## 状态

已采纳。

## 背景

玩家从客户端进入空房后，手动启动多个独立机器人进程成本过高，也容易因 URL、会话令牌或房间状态不同步失败。玩家视角需要“一键开打”，服务端则需要保持房间编排与规则推进仍由本仓库自有代码负责。

## 决策

- 客户端协议追加 `AddBotReq/AddBotResp`，`AutoMatchRequest` 追加 `pad_with_bots`；不引入 `RematchReq`，结算后继续复用既有 `ReadyReq`。
- bot 用户 ID 使用 `bot:<room_id>:<seat>`，与真人用户命名空间隔离。
- 本批优先交付单进程 / 同进程 AddBot 能力：lobby 分配 bot 座位，room 以普通用户身份占座；完整出牌 supervisor 与跨进程 cluster transport 作为后续增量完善。
- runtime 配置使用 `runtime.lobby.bot_supervisor_enabled` 与 `runtime.lobby.max_bots_per_room` 作为总闸和上限，避免误创建过多 bot。

## 后果

- 玩家可以从客户端请求补位机器人，快速开始时也可以要求服务端自动补齐。
- 当前实现先保证协议与座位编排闭环；如果未来需要真正托管出牌，应在该 ADR 下扩展 supervisor 事件消费和动作提交，而不是让客户端直接模拟多个真人会话。
- 分进程集群中的 bot transport 需要补充鉴权与路由设计，在未完成前应保持关闭或仅返回明确错误。
