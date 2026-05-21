---
title: 房间 deadline 单一所有权与 PhaseUpdate/PhaseToken 契约
status: accepted
date: 2026-05-13
---

# ADR-0045 房间 deadline 单一所有权与 PhaseUpdate/PhaseToken 契约

## 状态

已采纳。

## 背景

四川血战联调出现的三类高频现象在同一根因下汇聚：

1. 换三张玩家提交被拒后，定缺阶段倒计时立刻显示 `00s`，约 15 秒后服务端"瞬间"自动定缺。
2. 每次轮到本家的上家结束出牌时，本家倒计时直接归零，到点后服务端自动替本家弃牌。
3. 极个别提交报 `exchange tile from hand: tile not in hand`，无法在正常路径解释。

代码层根因清晰：

- `RoundState.deadlineUnixMs` 的写入点只在 `internal/service/room/scheduler.go:reset()` 内；调用方仅有 `internal/service/room/actor.go` 的 `a.resetScheduler()`。
- 当时 engine 在 `applyExchangeThree`、`ApplyDiscard`、`drawForCurrentTurn` 等位置改完阶段状态后立即 `progress := rs.roundProgress()` 取 `rs.deadlineUnixMs`，并把它编码进换三张/定缺完成通知、`ActionNotify`、`DrawTileNotify`、`ClaimPromptNotify` 等多类对外消息。当前开局完成通知已统一为 `OpeningDoneNotify`，旧专属完成通知不再是公共协议或运行入口。
- actor 派发模板恒为 `tr := a.doXxx(...) ; a.resetScheduler() ; reply`。即先编码 outgoing notify（携带的 deadline 仍是上一阶段旧值，已快过期或已过期），再调度新 deadline。
- 客户端把这些消息携带的旧 deadline 直接渲染成倒计时，导致 `00s` 显示与服务端真实计时长期错位；玩家迟到的请求落在 race 窗口内，落到错位的 hand 上，触发 `tile not in hand` 等家族化错误。

ADR-0044 已把房间状态拆为 `RoomLifecycle / RoundProgress / SeatRoster / RoundFacts / UXTransient` 五类事实并要求同源投影，但未规定 deadline 字段的写入所有权，也未约束"客户端请求必须绑定它所认知的局内阶段"。本 ADR 在 ADR-0044 基础上补齐这两条契约，从结构上根除上述三现象族。

## 决策

1. **deadline 单一所有权落在 engine**。`RoundState` 删除 `deadlineUnixMs` 字段，改为持有 `phaseStartUnixMs int64` 与 `phaseReason WaitingReason` 两个最小事实；`Deadline()` 是这两个字段经 `cfg.DurationFor(reason)` 计算的派生量。任何对外投影（`RoundProgress`、`PhaseUpdate`、持久化、`SnapshotNotify`）都只读 `Deadline()`，不允许独立写入 deadline。
2. **`enterPhase(reason)` 是 phaseReason 与 phaseStartUnixMs 的唯一写入入口**。当前实现把所有开局步骤收口为 `ReasonOpening`，具体开局动作只在 `waiting_action` / `available_actions` 中投影；claim、tsumo、discard 等等待态也必须经 `rs.enterPhase(reason)` 完成。该 helper 内部用注入的 `clock.Clock` 设置 `phaseStartUnixMs = clk.Now().UnixMilli()`。enforcer 在 `make verify-fast` 中静态校验。
3. **engine `Apply*` 返回 `PhaseTransition` 事务描述符**。结构形如 `{ From, To WaitingReason; StartUnixMs, DeadlineUnixMs int64; Notifications []Notification }`。Notifications 内嵌的 `PhaseUpdate` 在事务构造时一次性写入正确 deadline，不再存在"事后回填"路径。
4. **scheduler 退化为对齐 OS 定时器的副本**。删除 `durationFor()`；`reset(rs)` 改为 `armUntil(deadlineUnixMs int64)`，仅做 `clk.AfterFunc(deadline - now, fire)`，不读不写 `rs` 业务字段。actor 模板改为 `tr, err := a.doXxx(...) ; if err == nil { a.scheduler.armUntil(tr.DeadlineUnixMs) } ; reply`。
5. **协议层引入 `PhaseUpdate` 嵌入消息作为客户端唯一的 phase/deadline 接收口**。`PhaseUpdate { Phase phase; int64 step; WaitingReason reason; int64 deadline_unix_ms; int64 server_now_unix_ms; repeated string available_actions; int32 acting_seat }` 必须嵌入所有 `*Notify` 与所有动作 `*Response`（含成功、失败、`PHASE_DRIFTED`、snapshot 与 remote gate 透传路径）。客户端 reducer 删除散点的 `v.DeadlineUnixMS = ...` 写入，合并为单一 `applyPhaseUpdate(pu)`，所有 envelope 进 reducer 第一步即调它。
6. **客户端动作请求必须绑定 `PhaseToken{ step, reason }`**。所有可触发阶段切换的请求（`OpeningActionRequest / DiscardRequest / ChiRequest / PongRequest / GangRequest / HuRequest / PassRequest / ClaimRequest`）携带 `PhaseToken`。`ExchangeThreeRequest / QueMenRequest` 已废弃，当前开局提交统一走 `OpeningActionRequest.action`。actor 入口校验：`token.step != rs.step || token.reason != rs.phaseReason` 时返回新的错误码 `PHASE_DRIFTED` 并附完整 `PhaseUpdate`；客户端刷新 UI 并提示玩家按当前行动窗口重新操作，不再表达为"服务端已自动接管"。
7. **客户端倒计时按服务端时间计算**。`PhaseUpdate.server_now_unix_ms` 作为偏移基线，客户端以指数滑动平均 (`α = runtime.phase.skew_alpha`) 维护 `offset = serverNow - clientWallClock`，倒计时显示 = `deadline_unix_ms - (clientWallClock() + offset)`。本机时钟漂移、笔记本休眠、跨时区均不再影响显示。
8. **持久化对齐到新结构**。`engine_persist.go` 持久字段切为 `PhaseStartUnixMs + PhaseReason`，恢复时按 `clk.Now()` 重算 deadline；若已过期则立即向 actor mailbox 投递一次 `cmdAutoTimeout`，不再向客户端发出"已是过去时间"的 deadline。
9. **拒绝路径结构化日志强制**。engine 任一拒绝出口（`exchange not allowed / tile not in hand / pass not allowed / phase drifted` 等）必须打一行 INFO，字段包括 `seat / user_id / req_token / cur_step / cur_reason / hand_snapshot / req_payload / err`。
10. **proto 兼容窗**。第 5/6 条新增字段沿用 ADR-0012 兼容策略，旧 `deadline_unix_ms` 顶层字段保留一个发布窗后再删除并推进 `proto-baseline` 标签。

## 后果

- `RoundState.deadlineUnixMs` 与 `scheduler.durationFor` 整体消失；engine 与 scheduler 之间不再共享业务可变状态。
- actor 命令派发模板被简化为线性五步：取 token → 校验 → engine 执行 → arm 定时器 → 回复。scheduler reset 与通知编码的时序耦合不可能再被未来重构破坏。
- 客户端 reducer 中 `DeadlineUnixMS` 的三个写点（`state_apply.go` 的 draw/action/snapshot）合并为单一入口，新增任何接收事件都不会再出现"忘记更新倒计时"的状态。
- `tile not in hand` 这类家族化错误在 actor 入口层就被 `PHASE_DRIFTED` 替代，错误码语义清晰、可观测；CLI 真人路径只刷新当前窗口，bot/auto round 路径独立处理自动动作。
- 笔记本休眠、跨时区、容器与宿主时钟偏移不再影响客户端倒计时；可作为 SLO 观测项纳入 `metrics`。
- 重连恢复路径行为可预测，`TestRoomProcessRestartReplay` 这一类集成测试不再依赖客户端忽略"过期 deadline"的容忍度。
- 治理面：`.cursor/rules` 新增 `room-phase-owner.mdc` 约束 phaseReason 写入仅可经 `enterPhase`；`.build/config.yaml` 新增 `room.phase.token_required` 与 `room.deadline.single_owner` 治理键，由 enforcer 校验；`runtime.phase.skew_alpha` 进入 `internal/config.Config` 与 `configs/*.yaml`。
- 三现象专项 e2e（`TestPhaseDeadlineNotStale / TestDiscardDoesNotInheritStaleDeadline / TestExchangeRejectCarriesFreshDeadline / TestPhaseDriftedOnLateExchange / TestClockSkewCountdown`) 进入 player journey drill 回归矩阵。

## 相关

- [ADR-0012](0012-proto-baseline-and-versioning.md) Proto 基线与版本策略
- [ADR-0018](0018-room-timer-and-heartbeat-clock.md) 定时器与心跳时钟
- [ADR-0022](0022-runtime-knobs-and-storage-resilience.md) 运行时参数与存储弹性
- [ADR-0039](0039-sichuan-xuezhandaodi-authoritative-round-contract.md) 四川血战权威局内契约
- [ADR-0043](0043-root-cause-fix-policy.md) 根因优先的问题处理策略
- [ADR-0044](0044-room-state-and-client-contract.md) 房间状态与前后端交互契约
