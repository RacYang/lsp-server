# 摸打段 · draw_discard_segment

## 0. 上下文

- spec 节：§D 摸打循环、§Q 定缺（`[Q2.2]` 灰显部分）、§G 全局不变量（`[G2]` 输入许可分层）
- 涉及 AID：`A16`（摸打 cli 渲染与输入许可）
- 协议字段：`Envelope_DrawTile` / `Envelope_DiscardResp` / `Envelope_Action(action=discard)` / `RoundProgress.acting_seats / available_actions`

## 1. 条款对照表

| 条款 | 等级 | 预期帧/事实 | 修复前现象 | 修复后断言 | 结论 | AID/测试名 |
| --- | --- | --- | --- | --- | --- | --- |
| `[D1.1]` | MUST | `RoundPhase=PHASE_DRAW` 不写入 `WaitingAction="draw"` | `waitKindForRoundPhase(PHASE_DRAW)` 已返回 `"none"`；历史 commit `9736178` 已修过该 reducer 路径 | `TestPlayerJourney_D1_1_PhaseDrawDoesNotWriteWaitingAction` 显式投递 PHASE_DRAW 后断言 `WaitingAction != "draw"` | pass | `A16` / 同名测试 |
| `[D1.2]` | MUST | DrawTile 到达后本家手牌 +1，光标默认停在新摸牌位置 | applyDraw 把新牌 append+sortedTiles 即可入手；但 `cursor.SyncMode` 切到 Single 时把 Index 钉在 `handLen-1`，排序后新摸牌可能落中间 → 玩家要多按方向键 | 新增 `indexOfPendingDrawTile(view, handLen)`：在 SeatIndex==ActingSeat 且 PendingTile 非空时按 hand 反向查找索引；找不到回落 handLen-1 | pass | `A16` / `TestPlayerJourney_D1_2_DrawTileBringsNewTileIntoSelfHand` + `TestPlayerJourney_D1_2_CursorLandsOnFreshlyDrawnTile` |
| `[D1.3]` | MUST | 他家摸牌广播不携带明牌（per-seat privacy） | 服务端按座位投影后 tile="" | `TestPlayerJourney_D1_3_OtherSeatDrawHidesTile` 断言他家 Hand 仍为空、HandCnt +1 | pass | `A16` / 同名测试 |
| `[D2.1]` | MUST | 不满足出牌许可时 Enter 静默无效 | `submitCursorAction` 在 `cursor.Mode==None` 时仍走 `noticeInputRejected`，弹「当前阶段不能操作手牌」副作用 | `submitCursorAction` 入口加 nil-or-None 静默 return（详见正文 §3），None 模式不再弹通知 | pass | `A16` / `TestPlayerJourney_D2_1_EnterSilentWhenNotMyTurn` |
| `[D2.2]` | MUST | Enter 提交后该牌灰显 pending 且重复 Enter 不重复下发 | `cursor.SubmitAt` 已写入 Pending；`tileStyle` 已按 Pending 灰显；`CanSubmit` 在 Pending 下返回 false → submitCursorAction 二次进入直接 return | `TestPlayerJourney_D2_2_EnterDebouncedAfterSubmit` 连按两次 Enter，仅一次到达 Discard | pass | `A16` / 同名测试 |
| `[D2.3]` | MUST | DiscardResp 非 OK → 显示可读拒绝原因 + 恢复手牌 | reducer 已对 DiscardResp 非 OK 写入 `UXNotice="出牌失败: <reason>"`；cli 在 Enter 路径不预先移除手牌，故无需「恢复」 | 既有 `TestApplyDiscardRespFailureShowsNotice` 保留并通过 | pass | 既有测试 |
| `[D2.4]` | MUST | 出牌超时按 surrender 处理 | 服务端 engine_timeout 路径既有 surrender 入口；架构 gap A5（P0）已登记需对所有 available_actions 含 discard 的窗口做收口 | 本段 cli 侧无改动；登记由 `regression_verify` 阶段串到 A5 | deferred | A5 跟进 |
| `[D4.1]` | MUST | 已胡座位不参与摸打、显示「已胡」 | `seatStatusMark` 在 prep 段补了 `Hued → ✓`，band 同步 | A1 段已覆盖 | pass | `A1` 段既有测试 |
| `[D4.2]` | MUST | 牌墙剩牌数实时显示 | applyDraw / applyAction 都把 `v.WallRemaining = msg.GetWallRemaining()` 直接落桌 | 现状合规，本段不再补回归 | pass | 现状契约 |
| `[D5.1]` `[D5.2]` | MUST | 杠四形态 / 暗杠隐私 | 服务端投影 + cli 渲染待 drill；已登记 A8 | 本段不展开 | deferred | A8 跟进 |
| `[D6.1]` | SHOULD | 海底/杠上花等 ScoringContext 显式 | 待 settle_rematch 段验证；已登记 A9 | 本段不展开 | deferred | A9 跟进 |
| `[D7.1]` | MUST | 听牌权威下发 + TUI 提示 | `client.v1` 协议待补 `tenpai_by_seat`；已登记 A4 | 本段不展开 | deferred | A4 跟进 |
| `[Q2.2]` | SHOULD | 自家手牌缺门花色灰显 | `seat_tiles.go::drawSouthHand` 没有读 `QueBySeat`，缺门字段未驱动样式 | 新增 `isQueSuitTile(tile, que)` + `cursorHighlightedAt`，在 drawSouthHand 中给非高亮缺门 tile 套 `Foreground(ColorGray)` | pass | `A16` / `TestPlayerJourney_Q2_2_QueSuitTileDecodingAndCursorOverlay` |

## 2. 关键发现

- 现象（光标停位）：玩家摸一张靠左的牌，按花色排序后落到 hand 中部，光标却固定在最右端，导致玩家以为「没摸到」或必须连按方向键 — 这一步把「玩家心里期待的下一动作」打断。
- 现象（误击 Enter）：他家回合本应「Enter 完全没反应」，但 cli 反而弹 UXTransient「当前阶段不能操作手牌」，让玩家以为系统出错。
- 现象（缺门视觉）：定缺后玩家最大的认知摩擦是「我还有几张缺门要打」，TUI 给的提示只在顶栏文字带「缺 X」，自家手牌区从颜色上完全无法识别，违反 `[Q2.2]`「视觉上区分缺门花色」。
- 根因（光标）：`cursor.SyncMode` 把 mode 切入逻辑写死 `handLen-1`，没有把 `view.PendingTile` 这条「服务端权威的刚摸牌」字段引入定位。
- 根因（Enter 副作用）：`submitCursorAction` 的 disabled 分支不区分「玩家欠操作」与「玩家根本不在出牌窗口」，统一弹 UXTransient。
- 根因（缺门灰显）：seat_tiles.go 长期没有把 RoomView.QueBySeat 引入手牌渲染层，渲染只关心 cursor / Marked / Pending 三态。
- 锚点条款：`[D1.1]`、`[D1.2]`、`[D1.3]`、`[D2.1]`、`[D2.2]`、`[D2.3]`、`[Q2.2]`
- AID：`A16`

## 3. 修复跟踪

- [x] A16 / 光标定位：
  - `cmd/cli/discard_cursor.go` 新增 `indexOfPendingDrawTile(view, handLen)`，仅在 `SeatIndex==ActingSeat && PendingTile!=""` 时反向查找 hand 命中位置；找不到回落 `handLen-1`。
  - `cmd/cli/discard_cursor.go::SyncMode` 切入 Single 模式时优先用 `indexOfPendingDrawTile` 定位。
- [x] A16 / Enter 静默：
  - `cmd/cli/table_screen.go::submitCursorAction` 入口加 `cursor == nil || cursor.Mode == CursorModeNone` 静默 return，文档对齐 `[D2.1]` 「Enter 无效且不弹错误」。
- [x] A16 / 缺门灰显：
  - `cmd/cli/seat_tiles.go::drawSouthHand` 在 `tileStyle` 之后判定 `isQueSuitTile(tile, view.QueBySeat[seat])`，且通过 `cursorHighlightedAt` 让光标 / Marked 高亮位避让，再施加 `Foreground(ColorGray)`。
  - 新增 `cmd/cli/seat_tiles_test.go` 锁定 `isQueSuitTile` 与 `cursorHighlightedAt` 的全部花色 × que 状态组合。
- [x] 回归测试：上表 6 个 `TestPlayerJourney_D*` / `TestPlayerJourney_Q2_2_*` 全部新增并通过；既有 `TestApplyDiscardRespFailureShowsNotice` 等保留通过。
- [x] 演练复跑：`make verify-fast` 全绿。

## 4. 留底素材

- 代码改动集中在：`cmd/cli/discard_cursor.go`、`cmd/cli/table_screen.go`、`cmd/cli/seat_tiles.go`、`cmd/cli/seat_tiles_test.go`、`cmd/cli/state_apply_test.go`、`cmd/cli/table_screen_test.go`。
- 关键测试名（新增）：
  - `TestPlayerJourney_D1_1_PhaseDrawDoesNotWriteWaitingAction`
  - `TestPlayerJourney_D1_2_DrawTileBringsNewTileIntoSelfHand`
  - `TestPlayerJourney_D1_2_CursorLandsOnFreshlyDrawnTile`
  - `TestPlayerJourney_D1_3_OtherSeatDrawHidesTile`
  - `TestPlayerJourney_D2_1_EnterSilentWhenNotMyTurn`
  - `TestPlayerJourney_D2_2_EnterDebouncedAfterSubmit`
  - `TestPlayerJourney_Q2_2_QueSuitTileDecodingAndCursorOverlay`
- 与 `docs/spec/architecture-gaps.md` 的 A16 同步「已修复 + 测试名」。
- A4（tenpai 协议字段）、A5（超时收口）、A8（杠四形态）、A9（ScoringContext）作为后续段任务跟进。
