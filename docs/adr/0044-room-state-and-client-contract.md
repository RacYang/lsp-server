---
title: 房间状态与前后端交互契约
status: accepted
date: 2026-05-11
---

# ADR-0044 房间状态与前后端交互契约

## 状态

已采纳。

## 背景

房间生命周期、局内阶段、座位状态、客户端 UI 临时状态曾在不同层以相近字段名表达。典型表现包括：`state` 与 `phase` 的边界需要靠经验理解，`waiting_action` 被前端当作 UI 阶段使用，单进程 `client.v1` 事件比集群 `cluster.v1` 事件携带更多局内事实，重连快照与事件流在部分字段上无法证明等价。

这些问题不是单个分支错误，而是状态事实缺少统一命名和统一投影来源。若继续在 CLI、gate 或 bot 侧补特殊分支，会让本地模式、集群模式、重连恢复和机器人决策逐渐分叉。

## 决策

1. 将房间与局内状态拆成五类事实：
   - `RoomLifecycle`：房间宏观生命周期，仅来自 `internal/domain/room.FSM`，值域为 `waiting`、`ready`、`playing`、`settling`、`closed`。
   - `RoundProgress`：局内进度，仅由 `RoundState` 统一投影，包含 `phase`、`step` / `last_step`、`acting_seats`、`waiting_action`、`available_actions`、`claim_candidates`、`deadline_unix_ms` 与 `wall_remaining`。
   - `SeatRoster`：座位名册与座位状态，仅由服务端 `SeatInfo` 投影，包含 `seat_index`、`user_id`、`nickname`、`is_bot`、`online`、`auto_play`、`status`、`hand_count` 与 `total_score`。
   - `RoundFacts`：手牌、牌河、副露、最近动作、结算和规则元数据。
   - `UXTransient`：CLI 光标、pending、notice、布局焦点和中文文案等本地体验状态。
2. `state` 只表示 `RoomLifecycle`；`phase` 在 Go 内部命名为 `RoundPhase`，只表示局内阶段；`waiting_action` 只表示服务端正在等待哪类动作，不承载“摸牌中”等 UI 文案。
3. `PHASE_DRAW` 表示局内进度处于摸牌推进阶段，不是玩家可提交动作的等待态。客户端可据此显示“摸牌中”，但输入许可必须来自 `available_actions`、`acting_seats` 与本地光标状态。
4. room service 必须提供统一 projector；事件通知、`SnapshotRoom` / `SnapshotNotify`、bot `RoundView` 与持久化恢复都从同一投影获取 `RoundProgress`、`SeatRoster` 与 `RoundFacts`。
5. `client.v1` 与 `cluster.v1` 必须承载同一套状态事实。`LocalRoomGateway`、`remoteRoomGateway`、handler 与 room gRPC server 只做字段转换和路由，不拼业务事实，不创造 UI 状态。
6. lsp-cli reducer 按事实类型折叠事件：`applyRoundProgress`、`applySeatRoster`、`applyRoundFacts` 与 `UXTransient` 更新分离。动作事实不能顺手改下一焦点；下一阶段和焦点只由 `RoundProgress` 决定。
7. `SeatInfo.status` 先固化字符串值域；是否升级为 enum 另开协议演进。客户端不得改写服务端返回的 `SeatInfo`，例如不得把新增机器人本地改成 ready。
8. 对外 proto 兼容策略沿用 ADR-0012：现有字段不删除、不重编号、不改变 wire 语义；如需更名，先追加新字段并保留兼容期。

## 后果

- ADR-0039 的“客户端优先读取服务端 phase”升级为全链路状态事实同源要求。
- 集群路径不能再使用瘦身事件导致本地/集群下游语义不同；缺失字段必须在 `cluster.v1` 中追加并通过 gate 转回 `client.v1`。
- CLI 中 `draw` 只能作为 UX 派生结果存在，不能写入 `RoomView.WaitingAction`。
- AddBot、自动匹配、进房和重连恢复都必须以服务端 `SeatInfo` 更新座位状态，避免前端猜测 bot、ready 或托管状态。
- 测试必须覆盖本地与集群同源事件等价、重连 `last_step` 切点、`PHASE_DRAW` 派生、座位名册权威来源和 reducer 分层。

## 相关

- [ADR-0012](0012-proto-baseline-and-versioning.md) Proto 基线与版本策略
- [ADR-0014](0014-reconnect-session-and-snapshot-cutover.md) 断线重连、会话校验与快照回放切点
- [ADR-0039](0039-sichuan-xuezhandaodi-authoritative-round-contract.md) 四川血战权威局内契约
- [ADR-0043](0043-root-cause-fix-policy.md) 根因优先的问题处理策略
