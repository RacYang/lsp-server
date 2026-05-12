# 结算 + 再开一桌段 · settle_rematch_segment

## 0. 上下文

- spec 节：§S 结算、§R 再开一桌
- 涉及 AID：`A10`（多家胡 / 流局赔付 / 底栏键位）、`A18`（再开一桌 LeaveRoom→AutoMatch 顺序回归）；关联 `A6`（服务端零和断言，本段以 cli 侧护栏 + 服务端基线打桩，真正服务端 panic + 断言留给 regression_verify 阶段或单独服务端 PR）
- 协议字段：`SettlementNotify.{winner_user_ids,total_fan,seat_scores,penalties,per_winner_breakdown,total_scores}` / `WinnerBreakdown.{seat_index,user_id,fan,fan_names}` / `PenaltyItem.{reason,from_seat,to_seat,amount}` / `LeaveRoomReq` / `AutoMatchReq`

## 1. 条款对照表

| 条款 | 等级 | 预期帧/事实 | 现状 | 修复后断言 | 结论 | AID/测试名 |
| --- | --- | --- | --- | --- | --- | --- |
| `[S1.1]` | MUST | `SettlementNotify` 到达立即弹结算浮窗 | `state_apply.go::Envelope_Settlement` 写 `LastSettlement` + `RoomState=settling`，`SceneRouter` 由 Phase 推进进 `renderSettle` | 现状契约（既有 `TestApplyInitialDealDrawDiscardAndSnapshot` 系列覆盖 reducer 路径） | pass | 现状契约 |
| `[S1.2]` | MUST | 命令行同步输出文本摘要 | `WriteStdoutSummary` 在 lobby 切回时打印「本局摘要」段 | 既有 `TestWriteStdoutSummaryWin / Draw / NilWriterNoPanic` 覆盖 | pass | 既有测试 |
| `[S2.1]` | MUST | 累计积分取服务端 `total_scores`，不本地累加 | `state_apply.go::v.TotalScores = cloneSeatScores(...)` 直接拷贝服务端值 | 现状契约 | pass | 现状契约 |
| `[S2.2]` | MUST | 番种文案来自 `per_winner_breakdown.fan_names`，cli 不硬编码 | `snapshotSettlementSummary` 直接把 `breakdown.GetFanNames()` 投影到 `SettlementFan.Name` 与 `Winners[i].FanNames`；无本地字典 | `TestPlayerJourney_S3_1_MultiWinnerExpandsEachHuSeat` 顺带断言 FanNames 透传 | pass | `A10` / 同名测试 |
| `[S2.3]` | MUST | 胜负判定基于「本家 user_id 是否在 winner_user_ids」 | `snapshotSettlementSummary` 用 `containsString(winners, view.UserID)` 选 Outcome | `TestPlayerJourney_S3_1_*`（Win）+ `TestPlayerJourney_S4_1_*`（Draw） | pass | `A10` / 同名测试 |
| `[S3.1]` | MUST | 多家胡（一炮多响 / 血战连续胡）每家拆分清楚显示 | **修复前**：`snapshotSettlementSummary` 只把第一个 `breakdown` 写入 `WinnerID/Nick`，所有 fan 合并到一个 `Fans` 列表，玩家看不出每家归属。**修复后**：新增 `Winners []SettlementWinner`，逐家保留 nickname/fan/fan_names；`DrawSettlementDialog` 与 `WriteStdoutSummary` 都按家独立打印「胡 · X  N 番 · 番种列表」 | `TestPlayerJourney_S3_1_MultiWinnerExpandsEachHuSeat` 锁定逐家展开 + IsSelf 标记 | fixed | `A10` / 同名测试 |
| `[S4.1]` | MUST | 流局必须显式说明并按 `penalties` 显示查叫 / 花猪 / 退税 | **修复前**：`snapshotSettlementSummary` 丢弃 `notify.GetPenalties()`，浮窗与 stdout 都不展示赔付。**修复后**：新增 `Penalties []SettlementPenalty`，每条按 reason / from→to / amount 独立打印 | `TestPlayerJourney_S4_1_DrawPenaltiesAreSurfaced` 锁定 penalties 逐条投影 + 流局 Outcome | fixed | `A10` / 同名测试 |
| `[S5.1]` | MUST | 底栏键位「`r` 再开一桌 / `l` 离桌 / `Enter` 停留」 | **修复前**：`dialog_settlement.go::allLines` 末行写 `R 再来一局  /  L 离桌`（大写 + 文案错 + 缺 Enter）。**修复后**：替换为 `r 再开一桌  /  l 离桌  /  Enter 停留`，`renderSettle` 底栏文案早已一致 | `TestSettlementDialogIncludesAllScoresWhenRevealed` 已改为同时断言三段文案 | fixed | `A10` |
| `[S6.1]` | SHOULD | 流局后未胡座位的最终手牌按服务端权威投影亮出 | 取决于服务端 `SettlementNotify.detail_text` / 后续 SnapshotNotify；cli 不参与裁决 | 待 `A8/A9` drill 复核 | deferred | A9 |
| `[S7.1]` | MUST | 严格零和：seat_scores + penalties 代数和 = 0；不为零视为服务端缺陷 | cli 侧已加 `TestPlayerJourney_S7_1_SettlementZeroSum` 作为护栏 + 服务端基线；服务端 panic / 断言归 `A6` 跟进 | `TestPlayerJourney_S7_1_SettlementZeroSum` | pass（cli 侧）/ deferred（服务端） | `A6` / `A10` |
| `[R1.1]` | MUST | 按 `r` 必须先 `LeaveRoom` 真请求成功，再 `AutoMatch` | `main.go::restartAfterSettlement` 当前顺序为「本地 resetRoomToLobby → 调 gw.LeaveRoom（best-effort，忽略错误）→ 调 gw.AutoMatch」；本段以单元测试锁定 RPC 顺序，严格 reading「真请求成功」与「不得仅本地切场景」的张力先以注释 + A18 留痕，后续若用户认为强约束需要回切再单独提 PR | `TestPlayerJourney_R1_1_RestartIssuesLeaveThenAutoMatch` 锁定 `["leave","automatch"]` 顺序 + 落到新 RoomID | locked | `A18` / 同名测试 |
| `[R1.2]` | MUST | AutoMatch 不匹回原房（原房 state=settling/closed） | 与 `[L3.1]` 共用：服务端 lobby 路由跳过非 waiting 房；cli 仅复用 AutoMatchReq | 既有 `TestPlayerJourney_L3_1_*` + 本段 R1.1 用例间接断言 | pass | 既有 + `A18` |
| `[R1.3]` | MUST | `max_hands>1` 时由服务端按 ROOM-FSM 推进 | cli 仅 reducer 投影 RoomState/Phase，无本地推进 | 现状契约 | pass | 现状契约 |
| `[R1.4]` | MUST | 多局之间累计积分保留；ready/surrendered 被清除时顶栏短暂提示「准备开始下一局」 | `state_apply.go` 在 StartGame 时清 `LastSettlement` 与本局 ready 状态，但顶栏「准备开始下一局」提示尚未落 UXTransient | 列为后续 `A19` 候选（多局衔接 UX），本段不动 | deferred | A19（候选） |
| `[R2.1]` | SHOULD | 按 `l` 回大厅保留昵称与最近私密房码 | `scene.go::handleSettleKey` 调 `leaveRoomFireAndForget(TableExitGameOver)` → `applyTableExit` → `resetRoomToLobby(_, true)` 保留昵称；私密房码已随 `Private=false` 重置（A3 决策） | 现状契约 | pass | 现状契约 |
| `[R2.2]` | SHOULD | 按 `Enter` 停留在结算页慢慢看番种 | `handleSettleKey` 只处理 r/R/l/L/q/Q，其它键 fall-through，结算页保持不动 | 现状契约 | pass | 现状契约 |
| `[R3.1]` | MUST | `allow_leave_during_play=true` 时离桌走 LeaveRoom + surrendered；cli 必须提示「离桌将判为弃局」 | 服务端 surrendered 决策属于 A5；cli 离桌弃局提示尚未在 `TableExitGameOver` 路径前置 confirm。本段不动 | 留作 `A20` 候选 | deferred | A20（候选） |
| `[R3.2]` | MUST | `allow_leave_during_play=false` 时拒绝离桌并给出可读原因 | 与 `[R3.1]` 同源，依赖 `LeaveRoomResp.error_code` 路由；cli reducer 已在 `Envelope_LeaveRoomResp` 非 OK 时写 UXNotice | 现状契约 | pass | 现状契约 |

## 2. 关键发现

- 多家胡（一炮多响 / 血战「连续胡」）在过去版本被默默压成「赢家=第一个 winner」+ 全 fan 合集，玩家看不出哪份番种是谁的。`A10` 把 `SettlementSummary` 改成「Winners + Penalties + 主 Fans 列表」并行展示后，浮窗与 stdout 两路同步暴露；只要回归测试守住 `Winners` 切片长度与 `Penalties` 内容，未来谁误改成「合并展示」会立即红。
- `[S5.1]` 底栏键位错字「R 再来一局」是典型「文案漂移」类 P2 隐患——既有玩家旅程驱动测试不抓字面就发现不了。这条修复把测试改成同时断言「`r 再开一桌` / `l 离桌` / `Enter 停留`」三段，等于把「键位 + 中文文案 + 大小写」一并钉死。
- `[R1.1]` 与现状实现的严格性张力（best-effort LeaveRoom）暂以 `A18` 形式留痕：服务端在 settling/closed 房上拒绝 LeaveRoom 是合法的，强卡此处反而会让玩家从结算页回不去；后续若产品决策改成「失败必须 surface」，再单独提 PR + 服务端兜底。
- `[S7.1]` cli 侧只能做「投影一致性」护栏；真正的零和断言必须在服务端结算路径加 panic / 测试 fail，归到 `A6`。`TestPlayerJourney_S7_1_SettlementZeroSum` 在 cli 包里既是「服务端 dump 的现成基线」，也是未来回归 batch 的入口（拿真实 `SettlementNotify` 喂入即可）。

## 3. 修复跟踪

- [x] 改 `cmd/cli/dialog_settlement.go`：新增 `SettlementWinner` / `SettlementPenalty`，浮窗与 stdout 摘要按家 / 按罚分独立打印；底栏文案改为 `r 再开一桌  /  l 离桌  /  Enter 停留`
- [x] 改 `cmd/cli/main.go::snapshotSettlementSummary`：填充 `Winners` 与 `Penalties`，保持已有 `Fans` 合集语义不变
- [x] 改 `cmd/cli/dialog_settlement_test.go`：将原「R 再来一局」断言换成三段键位 + 文案
- [x] 增 `TestPlayerJourney_S3_1_MultiWinnerExpandsEachHuSeat` / `TestPlayerJourney_S4_1_DrawPenaltiesAreSurfaced` / `TestPlayerJourney_S7_1_SettlementZeroSum` / `TestPlayerJourney_R1_1_RestartIssuesLeaveThenAutoMatch`
- [x] 跑 `make verify-fast` 全绿
- [ ] 服务端零和 panic + 断言：归 `A6`，留给 regression_verify 阶段或单独服务端 PR
- [ ] `R1.4` 顶栏「准备开始下一局」UX 提示：候选 `A19`
- [ ] `R3.1` 离桌弃局 confirm：候选 `A20`
