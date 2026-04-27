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

### 2.1 Phase 5.3 score ledger 与分摊口径

Phase 5.3 将 `RoundState.totalFanBySeat` 替换为结算流水 `ScoreEntry`。房间层只负责把胡牌与杠牌事实追加到流水；`sichuanxzdd.BuildSettlement` 在局末 fold 出座位总分、罚分、退税与 `per_winner_breakdown`。

胡牌分摊口径如下：

- 自摸：未胡的三家各向胡家支付本次番数。
- 点炮：放炮座位独自向胡家支付本次番数。
- 抢杠胡：补杠座位作为责任方，按点炮口径支付，并在番种分解中标记「包牌」。

杠分口径如下：

- 明杠、补杠：每个未胡对手向杠牌座位支付 1 分。
- 暗杠：每个未胡对手向杠牌座位支付 2 分。
- 退税：花猪玩家退回其已收杠分；流局无听玩家退回其已收暗杠分。

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
- `BuildSettlement` 已完成 Phase 5.3 的查叫、花猪、退税、杠分流水与 `per_winner_breakdown` 折叠；后续规则深化应继续通过 `rules.ScoreContext` 与 `ScoreEntry` 扩展，而不是在传输层拼接口径。
