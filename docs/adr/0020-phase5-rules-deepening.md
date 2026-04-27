---
title: Phase 5.3 血战规则深化
status: accepted
date: 2026-04-27
---

# ADR-0020 Phase 5.3 血战规则深化

## 状态

已采纳。

## 背景

Phase 5 已经完成房间引擎收敛、协议 baseline 重置与最小观测指标，但血战到底的结算口径仍停留在 MVP：查叫依赖粗略的 `total_fan`，胡牌分摊没有区分自摸与点炮，杠分缺退税，`cluster.v1.SettlementEvent.per_winner_breakdown` 没有端到端填充。

## 决策

### 1. 听牌与查叫

`internal/mahjong/hu` 提供 `TingTiles` 与 `IsTing`。查大叫只惩罚未胡、非花猪且无听牌的玩家；花猪仍按查花猪处理。

### 2. 统一 score ledger

`internal/service/room.RoundState` 使用 `[]sichuanxzdd.ScoreEntry` 保存胡分、杠分、退税和包牌事实。`room` 层只追加流水，不在传输层拼结算口径；`sichuanxzdd.BuildSettlement` 统一 fold 出 `SeatScore`、`PenaltyItem`、`WinnerBreakdown` 与可读摘要。

### 3. 分摊、退税与包牌

- 自摸：未胡三家各付本次番数。
- 点炮：放炮座位独付本次番数。
- 抢杠胡：被抢杠座位作为责任方，按点炮支付，并在番种分解中加入「包牌」。
- 花猪：退回其已收杠分。
- 流局无听：退回其已收暗杠分。

### 4. 番种深化

Phase 5.3 增补不依赖首巡上下文的番种：将对、暗刻、暗杠、双暗杠、杠上炮。天胡、地胡需要庄家与首巡语义，后续由 [ADR-0021](0021-phase5-4-dealer-and-advanced-fans.md) 以 Phase 5.4 独立落地。

### 5. 可观测性边界

规则包不得依赖 `internal/metrics`。局末罚分指标由 `internal/service/room` 观察 `BuildSettlement` 返回值后记录，保持 `mahjong_*` 包的纯规则边界。

## 后果

- 结算状态从单一 `total_fan_by_seat` 迁移到可审计流水；恢复 JSON schema 升级为 v2，并兼容旧快照的总分字段。
- `client.v1.SettlementNotify` 新增 `per_winner_breakdown`，与 `cluster.v1.WinnerBreakdown` 字段集保持一致。
- 客户端可以展示结构化番种分解；老客户端可忽略新增字段。
