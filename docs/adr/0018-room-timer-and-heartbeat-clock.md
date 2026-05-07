---
title: 房间定时器调度、心跳策略与可注入时钟
status: accepted
date: 2026-04-27
---

# ADR-0018 房间定时器调度、心跳策略与可注入时钟

## 状态

已采纳。

## 背景

Phase 4 已提供 `ApplyTimeout` 托管入口，但没有后台调度器；测试也只能通过直接调用服务方法推进超时。Phase 5 需要真实定时器，同时保证房间状态仍只在 actor 内串行修改。

## 决策

- 新增 `internal/clock.Clock`，生产使用 `realClock`，测试使用 `Fake.Advance` 推进时间。
- 每个 room actor 持有 `roomScheduler`。actor 在状态推进后按当前等待态重置定时器。
- 定时器回调只调用 `submitAutoTimeout` 向同一 mailbox 投递命令，不直接读写 `RoundState`。
- 托管产生的通知通过 `Service.SetAutoTimeoutHandler` 交给本地 Hub 或 room gRPC server 处理，沿用现有广播、持久化和事件流路径。
- 配置暴露 `room.timeout.exchange_three`、`que_men`、`claim_window`、`tsumo_window`、`discard`；未配置时使用 `15s / 15s / 3s / 3s / 15s`。
- `session.Hub` 记录 `lastHeartbeat`，可通过注入时钟关闭超时连接；关闭连接本身不直接改变房间 FSM。
- WebSocket 关闭后由 gate 安排 `runtime.room.surrender_after_offline` 延迟任务；超时前若 Hub 已重新注册同一用户与房间，则任务退出；否则向 room actor 投递 surrender 路径。该延迟与 `runtime.room.surrender_action_timeout` 分工不同：前者决定离线多久后接管，后者决定已 surrender 座位在局内动作窗口中的托管响应速度。

## 后果

- 时间相关测试可通过 `fakeClock` 控制，不再依赖真实 sleep。
- 定时器回调不持有房间状态，保持“每房一 actor 串行修改”的并发边界。
- 后续 5.5 的 mailbox 有界 channel 与限流可直接复用该定时器投递路径。
