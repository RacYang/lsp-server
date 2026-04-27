---
title: Phase 5 房间引擎收敛与结算边界
status: accepted
date: 2026-04-27
---

# ADR-0017 Phase 5 房间引擎收敛与结算边界

## 状态

已采纳。

## 背景

Phase 4 之前，项目存在两套相似的牌局引擎：

- `internal/service/game.Engine`：早期用于自动回放与端到端冒烟。
- `internal/service/room.Engine`：当前真实客户端交互主链路，承载 actor 串行状态、换三张、定缺、摸打、抢答、恢复 JSON 与结算推送。

两套引擎都能生成 `client.v1` 通知，并各自维护一份自动回放和结算逻辑。Phase 5 将补完血战多胡、点炮、抢杠、退税、查叫、杠分流水等规则；继续保留双引擎会导致规则与结算实现漂移。

## 决策

### 1. 退役 `internal/service/game`

删除 `internal/service/game` 包。自动回放能力统一保留在 `internal/service/room.Engine.PlayAutoRound`，并拆入 `engine_auto.go`。

后续测试若需要确定性回放，直接使用 `room.NewEngine(ruleID).PlayAutoRound`。

### 2. 结算归属规则包

当前 MVP 结算从 `room` 包迁移到 `internal/mahjong/sichuanxzdd.BuildSettlement`。`room` 引擎只负责在一局结束时收集运行态并调用规则包生成：

- `SeatScore`
- `PenaltyItem`
- 可读摘要

Phase 5.3.0 扩展 `HuContext` / `ScoreContext`，把以下字段纳入规则接口：

- `HuSource`：`tsumo` / `discard` / `qiang_gang` / `bu_gang`。
- `GangRecord`：杠牌座位、杠类型、牌、来源座位、责任座位与步骤。
- 和牌场况：`IsHaiDi`、`IsGangShangHua`、`Discarder`、`ResponsibleSeat`、`WallRemaining`。
- 计分场况：每家根牌、杠流水、自摸标记、缺门数组与剩余牌数。

当前 `sichuanxzdd.ScoreFans` 接受扩展后的结构但暂不消费，后续每条血战规则 PR 在这些字段上增量实现。

### 3. `room` 包仍是编排边界

`internal/service/room` 继续负责：

- 每房 actor 串行事件循环。
- `RoundState` 最小恢复状态。
- `client.v1` 通知封装。
- `cluster.v1` 上游映射所需的 room notification。

麻将规则、番型与结算口径不再复制在 `service/game` 或 `service/room` 内部。

## 后果

- 后续血战规则只需修改 `internal/mahjong/sichuanxzdd` 与 `rules` 上下文，不需要同步双引擎。
- `internal/service/room/engine_auto.go` 成为唯一自动回放入口。
- 当前 `BuildSettlement` 仍是 Phase 4 MVP 口径；完整查叫、花猪、退税、杠分流水将在 Phase 5.3 分 PR 扩展。
