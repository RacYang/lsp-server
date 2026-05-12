# 换三张 + 定缺段 · exchange_que_segment

## 0. 上下文

- spec 节：§E 换三张、§Q 定缺、§G 全局不变量（[G2] 输入许可分层）
- 涉及 AID：`A14`（换三张 UI/通知）、`A15`（定缺键位/文案）
- 协议字段：`Envelope_InitialDeal` / `Envelope_Action(action=exchange_three)` / `Envelope_ExchangeThreeResp` / `Envelope_ExchangeThreeDone` / `Envelope_QueMenResp` / `Envelope_QueMenDone`

## 1. 条款对照表

| 条款 | 等级 | 预期帧/事实 | 修复前现象 | 修复后断言 | 结论 | AID/测试名 |
| --- | --- | --- | --- | --- | --- | --- |
| `[E1.1]` | MUST | `waiting_action=exchange_three` 时本家手牌严格 13 张权威 | reducer 路径已能保留 13 张，但缺乏专项断言 | `TestPlayerJourney_E1_1_*`：投递 13 张 + Action(exchange_three)，断言手牌长度 13 且 13 张全部命中 | pass | `A14` / `TestPlayerJourney_E1_1_ExchangeThreeKeepsThirteenAuthoritativeTiles` |
| `[E1.2]` | MUST | 选第二张异花色时 UI 立即拒绝标记 | `table_screen.go` Space 处理只调 `cursor.ToggleMark()`，不识别花色，异花色会被本地标记并发到服务端 | `sameExchangeSuit(hand, marked, candidate)` 在 Space 前置校验；命中异花色 → `noticeInputRejected("换三张必须同一花色")` 并 `return` | pass | `A14` / `TestPlayerJourney_E1_2_ExchangeMarkRejectsCrossSuit` |
| `[E2.1]` | MUST | 底栏实时显示「已选 N/3」字面 | 原文案「还需 N 张」「已选 3 张」非字面 N/3 | `primaryPrompt` 改为「换三张 · 已选 N/3 · ←/→ 移动，Space 标记，Enter 提交（须同花色）」与「已选 3/3 · 按 Enter 提交换牌」 | pass | `A14` / `TestCentralPromptStates`（已更新） |
| `[E2.2]` | MUST | 服务端拒绝必须落到 UXTransient 并保留 Marked 让玩家改选 | `Envelope_ExchangeThreeResp` 只走 `appendResponseLog`，UXNotice 不触发 | reducer 在非 OK 时写 `UXNotice="换三张被拒绝：<reason>"` + 3s TTL；不清 Marked | pass | `A14` / `TestPlayerJourney_E2_2_ExchangeThreeRejectionSurfacesNotice` |
| `[E3.1]` | MUST | 服务端按 `RoundState.exchangeDirection` 执行，cli 不猜方向 | cli 端 `ExchangeThree(ctx, tiles, 0)` 直接传 0，由服务端按 RoundState 决定 | 现状已合规：cli 不复刻交换逻辑，仅渲染权威投影；本段无代码改动，登记为既有契约 | pass | n/a |
| `[E3.2]` | MUST | `ExchangeThreeDoneNotify` 后下一帧手牌按权威投影替换 | 既有 `TestApplyExchangeDone*` 已锁 reducer 路径 | 既有 `TestApplyExchangeDoneSwapsThreeTiles` + `TestApplyExchangeDoneUsesPendingAwayWhenProjectionOmitsAway` 保留为锚点 | pass | 既有用例 |
| `[Q1.1]` | MUST | 仅 m/p/s 三键提交定缺；其它键忽略且无副作用 | `1/2/3` 同样下发 QueMenReq；`m/p/s` 在非 que_men 阶段会弹「当前不能定缺」UXTransient | 删 `1/2/3` 分支；`m/p/s` 在不在白名单时直接 `return tableEventResult{}`，不写 notice | pass | `A15` / `TestPlayerJourney_Q1_1_QueMenKeysOnlyMPS` + `TestPlayerJourney_Q1_1_QueMenKeysSilentWhenNotAllowed` |
| `[Q1.2]` | MUST | QueMenDone 后 `view.QueBySeat` 全桌可见 | reducer 已落地，缺独立回归 | `TestPlayerJourney_Q1_2_QueMenDoneFillsRoster` 投递四家 que_suit_by_seat 后断言 view.QueBySeat 完整 | pass | `A15` / `TestPlayerJourney_Q1_2_QueMenDoneFillsRoster` |
| `[Q1.3]` | MUST | 全四家提交才进入摸打 | reducer 中 `applyRoundProgressFromPhase` 由服务端 round projector 驱动 | 现状由服务端 acting_seats 收口；cli 不本地推断，逻辑由集成测试覆盖 | pass | 既有集成测试 |
| `[Q2.1]` | SHOULD | 提示文案必须告诉玩家「选定后不可更改」 | 原文案「请定缺：1 缺万，2 缺筒，3 缺条」未提及不可更改 | model.Hint 改为「请定缺：m 缺万 / p 缺筒 / s 缺条（选定后不可更改）」；primaryPrompt 同步更新 | pass | `A15` / 文案验证由人工 drill 复核 |
| `[Q2.2]` | SHOULD | 自家手牌区视觉区分缺门花色 | 现状是否灰显待复核；本轮未触碰 hand 渲染层 | 留作 `draw_discard_segment` 一并验证（缺门灰显与 [D7.1] 听牌提示一并验收） | deferred | 后续段 |

## 2. 关键发现

- 现象（A14）：玩家在换三张阶段误选异花色不会被立刻阻止，要等服务端 round-trip 才能感知；且服务端拒绝原因只进 Log，主界面无任何提示。
- 现象（A15）：玩家在摸牌阶段误按 `m` / `p` / `s` 会突然弹出「当前不能定缺」，污染主提示区；而 `1` / `2` / `3` 又被当作快捷键加深错觉，与 [Q1.1] 「其它键忽略」相违。
- 根因（A14）：换三张交互长期没有把「同花色」与「N/3 计数」当作 cli 层的硬约束，依赖服务端兜底；UXTransient 通道在 ExchangeThreeResp 路径上漏接。
- 根因（A15）：早期为方便键盘党加了数字键快捷方式，但与 [Q1.1] 的「白名单 = m/p/s」相冲突；non-allowed 路径误用了 `noticeInputRejected`，把「按错键」错当成「不能定缺」的功能性拒绝。
- 锚点条款：`[E1.1]`、`[E1.2]`、`[E2.1]`、`[E2.2]`、`[Q1.1]`、`[Q1.2]`、`[Q2.1]`
- AID：`A14`、`A15`

## 3. 修复跟踪

- [x] A14 / 换三张 UI 同花色 + 文案 + 通知：
  - `cmd/cli/table_screen.go` Space 键加 `sameExchangeSuit` 前置校验，异花色弹「换三张必须同一花色」UXTransient 并 `return`。
  - `cmd/cli/table_screen.go` 新增 `sameExchangeSuit(hand, marked, candidate)` 辅助函数；仅识别 m/p/s 三种花色，z 字牌作为规则边界自动拒。
  - `cmd/cli/interaction.go` primaryPrompt Exchange 分支文案改为 `已选 N/3 · ←/→ 移动 ...`，满 3 张换「已选 3/3 · 按 Enter 提交换牌」。
  - `cmd/cli/state_apply.go` `Envelope_ExchangeThreeResp` 在 error_code 非 OK 时把 `envelopeError` 结果写入 `v.UXNotice = "换三张被拒绝：<reason>"`，TTL 3s；不清 cursor.Marked。
- [x] A15 / 定缺键位 + 文案：
  - `cmd/cli/table_screen.go` 删除 `'1' '2' '3'` 三个 case；`'m' 'M' 's' 'S'` 与「碰」之外的 `'p' 'P'` 在 `!containsAction(QueMen)` 时 `return tableEventResult{}`，不再写 noticeInputRejected。
  - `cmd/cli/interaction.go` Hint 与 primaryPrompt 改为「m 缺万 / p 缺筒 / s 缺条（选定后不可更改）」。
  - `cmd/cli/state_apply.go` `Envelope_QueMenResp` 在 error_code 非 OK 时写 `v.UXNotice = "定缺被拒绝：<reason>"`，TTL 3s。
- [x] 回归测试：上表中所有 `TestPlayerJourney_*` 全部新增；既有 `TestApplyExchangeDoneSwapsThreeTiles` / `TestApplyExchangeDoneUsesPendingAwayWhenProjectionOmitsAway` / `TestCentralPromptStates` 保留并保持通过。
- [x] 演练复跑：`make verify-fast` 全绿。
- [ ] 真实端到端复跑：交由下一段触发或人工 `scripts/drills-dev.sh start --cli`。

## 4. 留底素材

- 代码改动集中在：`cmd/cli/table_screen.go`、`cmd/cli/interaction.go`、`cmd/cli/state_apply.go`、`cmd/cli/table_render_test.go`、`cmd/cli/table_screen_test.go`、`cmd/cli/state_apply_test.go`。
- 关键测试名（新增）：
  - `TestPlayerJourney_E1_1_ExchangeThreeKeepsThirteenAuthoritativeTiles`
  - `TestPlayerJourney_E1_2_ExchangeMarkRejectsCrossSuit`
  - `TestPlayerJourney_E2_2_ExchangeThreeRejectionSurfacesNotice`
  - `TestPlayerJourney_Q1_1_QueMenKeysOnlyMPS`
  - `TestPlayerJourney_Q1_1_QueMenKeysSilentWhenNotAllowed`
  - `TestPlayerJourney_Q1_2_QueMenDoneFillsRoster`
- `docs/spec/architecture-gaps.md` 同步新增 A14 / A15 条目并标「已修复 + 测试名」。
