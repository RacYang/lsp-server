# 房间 FSM 与局内进度（Phase 7）

## 房间生命周期

房间 FSM 只描述 `RoomLifecycle`，遵循 [ADR-0044](adr/0044-room-state-and-client-contract.md) 的状态分层。它不承载换三张、定缺、摸牌、出牌、抢答或结算明细等局内语义。

- `idle`：占位，当前实现从 `waiting` 起步。
- `waiting`：等待玩家进房与准备。
- `ready`：四人已满且全准备，可开局。
- `playing`：对局进行中。具体局内阶段由 `RoundProgress` 表达。
- `settling`：结算中。
- `closed`：房间关闭。

## 局内进度

局内进度由 `internal/service/room.RoundState` 统一投影为 `RoundProgress`，不是房间 FSM 的子状态。`RoundProgress` 包含 `phase`、`step` / `last_step`、`acting_seats`、`waiting_action`、`available_actions`、`claim_candidates`、`deadline_unix_ms` 与 `wall_remaining`。

对局进入 `playing` 后，局内推进由当前 `rule_id` 的规则策略驱动。room 只维护通用等待态、行动窗口、`win_events`、`score_events`、opaque `rule_state` 与投影调度；换三张、定缺、首胡终局、血战续行等都不是房间 FSM 的内建阶段。

- 若规则声明 `OpeningPolicy`，服务端以 `reason=opening` 进入通用开局等待，具体动作由策略投影为 `waiting_action`；四川血战会投影为 `exchange_three`、`que_men`。开局提交统一走 `opening_action`，步骤完成统一投影为 `opening_done`；无开局流程的规则直接进入摸打。
- 客户端可以把四川 action 渲染成“换三张”“定缺”，但不得把它们建模成独立局内 phase；本地输入、倒计时和焦点都应从 generic opening + `available_actions` 派生。
- 所有开局步骤统一投影为 `phase=opening`；不得为换三张、定缺或未来开局动作新增专属 phase。
- 当前座位摸牌后进入 `等待出牌`。
- 若摸牌立即可自摸，则先进入 `等待自摸决策`，客户端可发送 `hu_req` 或对摸到的牌发送 `discard_req`。
- 最近一次弃牌会保留一个抢答窗口；可抢座位收到 `hu_choice` / `pong_choice` / `gang_choice` / `qiang_gang_choice` 后，可发送对应请求中断当前待出牌座位。
- 玩家胡牌后写入通用 `WinEvent` / `ScoreEvent`；是否立即结算、胡后退出轮转、继续参与或等待多响，都由 `TurnFlow` / `TerminationPolicy` 决定。四川血战会投影兼容的已胡座位状态，但 `hued_seats` 不是 room 主模型。

`PHASE_DRAW` 表示服务端正在推进摸牌或即将下发摸牌事件，客户端可显示“摸牌中”；它不代表玩家可提交动作。是否允许输入只看 `acting_seats`、`available_actions` 与候选窗口。

`waiting_action` 只表示服务端正在等待哪类动作，合法值来自规则策略和通用动作管线。`exchange_three`、`que_men` 是四川血战等规则的协议兼容动作名，不是通用 room 阶段；`discard`、`claim_window`、`tsumo_window`、`none` 或空值用于通用摸打与等待态。不得用 `draw` 表示展示状态。

## 迁移

详见 `internal/domain/room/fsm.go` 中的显式迁移表；非法迁移会返回错误，避免静默破坏房间一致性。

结算后支持两种收尾：

- 默认 `max_hands=1`：`playing -> settling -> closed`，保持旧行为。
- 多局房间：`playing -> settling -> waiting`，保留座位与房间级累计积分，清理本局 ready / surrendered 状态，玩家重新准备后进入下一局。达到局数上限或解散时进入 `closed`。

## 超时策略

- `waiting`：超时策略仍为工程占位，当前不自动踢人；运维侧可直接回收长时间空房。
- `ready`：若未凑齐四人全准备，房间停留在 `ready` 前的等待阶段；当前不自动回退准备态。
- `playing`：
  - 出牌/自摸待决超时时，真人座位只能判 `surrendered` 并退出后续摸打轮转，不得由服务端代选弃牌或胡牌。
  - 抢答窗口超时时，真人座位只能显式 `pass` 或判 `surrendered`，不得由服务端按优先级替玩家选择胡/杠/碰。
  - 机器人与自动回放可使用独立 bot/auto round 路径做确定性动作，不复用真人 timeout 语义。
  - 后台定时器通过可注入 `clock.Clock` 调度；开局统一使用 `opening` 超时配置，并可按 `waiting_action` 覆盖，摸打仍按 `claim_window`、`tsumo_window` 与 `discard` 覆盖。

## 重连与恢复

- `SnapshotRoom` 返回房间玩家、规则投影、阶段、快照游标以及等待态摘要（谁可操作、等待什么动作、候选动作）。定缺、换三张提交态等只来自 `RuleStateProjector` 的兼容投影。
- `SnapshotRoom` / `SnapshotNotify` 同步返回结构化副露、最近动作、权威剩牌数、当前 deadline、局号、累计积分与规则元数据；客户端不应从旧字符串日志反推这些事实。
- `room` 冷启动可基于 `snapmeta.round_json` 恢复进行中的局面；`round_json.schema_version` 高于当前版本时降级到重新准备。
- 过牌后的 `claim_window_open=false` 不会再凭 `LastDiscard` 重新打开抢答窗口。
- 幂等仍以 `ApplyEvent.idempotency_key` 为入口，Redis 只记录请求是否已成功落地，不重放业务副作用。
- Phase 6 恢复演练以 `SnapshotRoom` 和 `StreamEvents` 可读性作为 PostgreSQL 恢复后的核心校验点。
