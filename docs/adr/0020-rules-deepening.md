---
title: Phase 5.3 血战规则深化
status: accepted
date: 2026-04-27
---

# ADR-0020 Phase 5.3 血战规则深化

## 状态

已采纳。

## 当前实现状态

本 ADR 是四川血战规则深化的历史决策。ADR-0040 后，`room` 不再保存历史四川计分条目作为主运行模型；胡牌、杠分、退税和包牌事实统一落到通用 `ScoreEvent`，再由四川规则包的结算策略折叠。

## 背景

Phase 5 已经完成房间引擎收敛、协议 baseline 重置与最小观测指标，但血战到底的结算口径仍停留在 MVP：查叫依赖粗略的 `total_fan`，胡牌分摊没有区分自摸与点炮，杠分缺退税，`cluster.v1.SettlementEvent.per_winner_breakdown` 没有端到端填充。

## 决策

### 1. 听牌与查叫

`internal/mahjong/hu` 提供 `TingTiles` 与 `IsTing`。查大叫只惩罚未胡、非花猪且无听牌的玩家；花猪仍按查花猪处理。

### 2. 统一 ScoreEvent

历史实现中，这里曾以四川私有计分条目表达胡分、杠分、退税和包牌事实。当前实现改为通用 `ScoreEvent`；`room` 层只追加事实，不在传输层拼结算口径，四川规则包统一 fold 出 `SeatScore`、`PenaltyItem`、`WinnerBreakdown` 与可读摘要。

### 3. 分摊、退税与包牌

- 自摸：未胡三家各付本次番数。
- 点炮：放炮座位独付本次番数。
- 抢杠胡：被抢杠座位作为责任方，按点炮支付，并在番种分解中加入「包牌」。
- 花猪：退回其已收杠分。
- 流局无听：退回其已收暗杠分。

### 4. 番种深化

Phase 5.3 增补不依赖首巡上下文的番种：将对、暗刻、暗杠、双暗杠、杠上炮。天胡、地胡需要庄家与首巡语义，后续由 [ADR-0021](0021-dealer-and-advanced-fans.md) 以 Phase 5.4 独立落地。

### 5. 可观测性边界

规则包不得依赖 `internal/metrics`。局末罚分指标由 `internal/service/room` 观察 `BuildSettlement` 返回值后记录，保持 `mahjong_*` 包的纯规则边界。

## 后果

- 结算状态从单一 `total_fan_by_seat` 演进为可审计的通用 `ScoreEvent`。当前持久化已硬切到通用 schema，不再兼容旧快照的总分字段。
- `client.v1.SettlementNotify` 新增 `per_winner_breakdown`，与 `cluster.v1.WinnerBreakdown` 字段集保持一致。
- 客户端可以展示结构化番种分解；老客户端可忽略新增字段。
