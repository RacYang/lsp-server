---
title: Phase 5 协议基线重置与恢复快照版本化
status: accepted
date: 2026-04-27
---

# ADR-0016 Phase 5 协议基线重置与恢复快照版本化

## 状态

已采纳。

## 背景

Phase 4 已把真实客户端动作接入 `gate -> room.ApplyEvent -> actor -> StreamEvents` 主链路，但仍存在三类契约缺口：

1. `client.v1.SettlementNotify` 已包含 `seat_scores` 与 `penalties`，而 `cluster.v1.SettlementEvent` 只传赢家、总番与文本摘要，跨进程订阅方会丢失结构化结算。
2. 重连快照只暴露单个 `acting_seat` 与 `available_actions`，无法表达多个仍有效的抢答候选。
3. Redis `snapmeta.round_json` 缺少 schema 版本。Phase 5 将改造 `RoundState` 以支持血战多胡、点炮、抢杠、杠分流水等，旧快照若没有版本边界，冷启动时可能让 `recoverOwnedRooms` 整批失败。

这些变更会影响 proto 基线。Phase 5 选择一次性重置 `proto-baseline`，避免在后续业务规则 PR 中反复制造 breaking noise。

## 决策

### 1. `cluster.v1.SettlementEvent` 与客户端结算对齐

`cluster.v1.SettlementEvent` 追加：

- `seat_scores`
- `penalties`
- `per_winner_breakdown`

`room` 进程在 `mapNotificationToEvent` 中从完整 `client.v1.SettlementNotify` 映射到 cluster event；结算落库也从 cluster event 反向恢复完整 `SettlementNotify`，不再只保存 `winner_user_ids`、`total_fan` 与 `detail_text` 子集。

### 2. 快照显式暴露抢答候选

`client.v1.SnapshotNotify` 与 `cluster.v1.SnapshotRoomResponse` 追加 `claim_candidates`。

该字段表达恢复切点上仍然有效的候选座位与动作集合。Phase 5.3 引入点炮胡、过牌与抢杠胡后，客户端可用该字段恢复完整选择面，而不只依赖 `acting_seat`。

### 3. `Envelope` 增加幂等键

`client.v1.Envelope` 追加 `idempotency_key`，用于 Phase 5.5 将本地 WebSocket 路径与集群 `ApplyEvent.idempotency_key` 对齐。空值表示不启用请求去重。

### 4. `round_json` 引入 schema 版本

`internal/service/room.roundPersist` 追加 `schema_version`，首版为 `1`。

恢复策略：

- 当前服务识别 `schema_version <= 1` 的快照。
- 遇到未来版本时返回 `ErrRoundPersistUnsupportedSchema`。
- `cmd/room` 冷启动恢复捕获该错误后，将房间降级为 `ready`，等待玩家重新准备，而不是让整个 room 进程启动失败。

### 5. proto baseline 重置

本阶段允许移动 `refs/tags/proto-baseline` 一次。移动动作必须和本 ADR、生成物、`CHANGELOG.md` 的 BREAKING 说明一起进入同一个交付批次。

## 后果

- 旧客户端若严格按旧 `cluster.v1.SettlementEvent` 结构消费，需要同步升级。
- 未来 `round_json` 结构演进有了显式边界；未知版本优先降级到可重新准备，不阻断进程就绪。
- 后续 Phase 5.3 的规则补完可复用 `claim_candidates` 与完整结算结构，不再频繁改变协议骨架。
