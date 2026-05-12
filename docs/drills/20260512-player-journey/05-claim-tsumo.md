# 抢答 + 自摸窗口段 · claim_tsumo_segment

## 0. 上下文

- spec 节：§C 抢答（碰/杠/胡/过）、§T 自摸窗口
- 涉及 AID：`A17`（claim / tsumo cli 回归补齐），关联 `A5`（超时收口在服务端 engine_timeout，本段不动）
- 协议字段：`RoundProgress.claim_candidates` / `available_actions` / `deadline_unix_ms` / `Envelope_ActionNotify(hu_choice|pong_choice|gang_choice|tsumo_choice)` / `Envelope_PassResp`

## 1. 条款对照表

| 条款 | 等级 | 预期帧/事实 | 现状 | 修复后断言 | 结论 | AID/测试名 |
| --- | --- | --- | --- | --- | --- | --- |
| `[C1.1]` | MUST | 抢答候选完全由 `RoundProgress.claim_candidates / available_actions` 驱动 | `interaction.go::claimActionsForSeat` 严格读 `view.ClaimCandidates` 后回退 ActingSeat + AvailableActions；无 LastDiscard 反推路径 | 现状合规；与 [C1.2] 共用回归 | pass | 现状契约 |
| `[C1.2]` | MUST | 弹窗仅对 `claim_candidates` 列出的座位呈现 | 同上：非 candidate 的 SeatIndex 拿到空 actions → InteractionModel.Allowed 空 → buildClaimDialog 不被调用 | `TestPlayerJourney_C1_2_ClaimDialogOnlyForCandidates` 双向断言 | pass | `A17` / 同名测试 |
| `[C2.1]` | MUST | 弹窗倒计时来自 `deadline_unix_ms` | `applyAction` 把 `action.GetDeadlineUnixMs()` 写入 `v.DeadlineUnixMS`，`ClaimDialogState.Progress` 按 `Deadline-OpenedAt` 换算 | 既有 `TestClaimDialogProgress` 等覆盖 | pass | 既有测试 |
| `[C2.2]` | MUST | 抢答窗口超时由 cli 显式发 `PassRequest`；离线时服务端判 surrendered | cli 侧 ticker 已在 `claimDialog.Expired(now)` 时调 `gateway.Pass`；服务端 surrendered 路径属于 A5 | cli 路径在自动 pass 单元已有 `TestClaimDialogExpired*` 覆盖；A5 跟进服务端 | pass（cli 部分） | 既有 + A5 后续 |
| `[C3.1]` | MUST | 键位 `←→` 切按钮、`Enter` 确认、`h/g/p/n` 直选 | `table_screen.go` h/g/p/n 已映射到 submitClaimAction(Hu/Gang/Pong/Pass) | 既有 `TestHandleTableKeyClaim*` 覆盖 | pass | 既有测试 |
| `[C3.2]` | MUST | 过期窗口不得在客户端重新打开 | `redraw()` 中 `model.Claim==nil → claimDialog=nil`；reducer 在 PassResp OK 时清 `ClaimCandidates={}`，下一帧 InteractionModel.Allowed 自然变空 | 状态机层面已无重开路径；本段不再补回归（loop 行为难以单元化） | pass | 现状契约 |
| `[C3.3]` | MUST | 显式过 / 超时兜底都必须发 `PassRequest`，不依赖服务端默认 | `submitClaimAction` 的 `default` 分支落在 `gateway.Pass(ctx)`；ClaimActionPass 与未知 action 都走该路径 | `TestPlayerJourney_C3_3_ExplicitPassSendsPassRequest` 覆盖显式 Pass 与未知 action fallback 两条路径 | pass | `A17` / 同名测试 |
| `[C4.1]` | MUST | 接力抢答按胡>杠>碰优先级显示候选 | 优先级由服务端 round projector 决定后下发 `ClaimCandidates`；cli 不参与裁决 | 现状契约 | pass | 现状契约 |
| `[C4.2]` | SHOULD | 一炮多响弹窗对每个可胡者独立 | 服务端为每个胡者下发独立 hu_choice，每家自取 `ClaimCandidates[selfSeat]` | 现状契约 | pass | 现状契约 |
| `[C5.1]` | MUST | 碰 / 杠 成功后必须等本家显式 DiscardRequest | `applyAction` 在 pong 成功后写 WaitingAction=discard，cli 不伪造下一动作 | 现状契约 | pass | 现状契约 |
| `[C5.2]` | MUST | 抢杠胡 +1 番并使被抢明杠无效 | 服务端结算计算；cli 只读 SettlementNotify | A9 / S2.2 跟进 | deferred | A9 |
| `[C5.3]` | MUST | 抢杠胡的被抢者必须看到「杠被抢」 | 取决于服务端 `ActionNotify.detail` 字段是否带专用 reason | 待 A9 / A10 drill 复核 | deferred | A9 |
| `[T1.1]` | MUST | 摸牌后立即可胡时进 tsumo_window 弹窗 | `applyAction(tsumo_choice)` 写入 `WaitingAction=tsumo_window`，AvailableActions=`[hu, pass]` | 既有 reducer 路径合规 | pass | 现状契约 |
| `[T1.2]` | MUST | 不胡后摸到的牌入手并继续 discard | PassResp OK on tsumo_window → reducer 推进到 PHASE_DISCARD + WaitingAction=discard + PendingTile 清空（hand 已在 applyDraw 时收纳） | `TestPlayerJourney_T1_2_PassOnTsumoReturnsToDiscard` | pass | `A17` / 同名测试 |
| `[T2.1]` | MUST | 自摸窗口倒计时同源 `deadline_unix_ms` | 与 [C2.1] 共用通道 | 现状契约 | pass | 现状契约 |
| `[T2.2]` | SHOULD | 默认建议高亮「胡」按钮 | buildClaimDialog 把 Allowed=[Hu, Pass] 转成 Actions=[Hu, Pass]，`SelectedIndex=0` → Selected=Hu | `TestPlayerJourney_T2_2_TsumoDialogDefaultsToHu` | pass | `A17` / 同名测试 |
| `[T3.1]` | MUST | 海底/杠上等 ScoringContext 服务端标记 | 待 A9 drill 复核 | 本段不动 | deferred | A9 |
| `[T4.1]` | MUST | 暗杠后摸牌走同源 tsumo_window | 服务端走相同 tsumo_choice 路径；他家视角 tile 由网关抹除 | 待 A8 一并跟进 | deferred | A8 |

## 2. 关键发现

- 抢答 / tsumo 在 cli 这一侧的状态机长期已经按权威字段驱动；spec 0.5 把它们从「可能存在的隐患」明确升为 MUST 后，最大的缺口是「没有独立测试把这些不变量钉死」。本段主要工作是补回归而非改实现。
- 关键避雷点：`submitClaimAction` 的 default 分支若被误改成 `gateway.Hu` 这类"安全默认"，玩家点过会变成胡牌——这是 A17 的最高优先级守护。`TestPlayerJourney_C3_3_*` 用「未知 action」用例锁住这条护栏。
- `[C2.2]` / `[T1.2]` 离线时「服务端直接判 surrendered」属于服务端 engine_timeout 路径的硬契约，与 spec [D2.4] 共用收口；本段不动服务端，留给 A5 跟进。

## 3. 修复跟踪

- [x] 回归补齐：
  - `cmd/cli/state_apply_test.go` 新增 `TestPlayerJourney_C1_2_*` / `TestPlayerJourney_T1_2_*` / `TestPlayerJourney_T2_2_*` 三条 reducer + interaction 层断言。
  - `cmd/cli/table_screen_test.go` 新增 `claimPassGateway` 工具类型 + `TestPlayerJourney_C3_3_*`，对显式 Pass 与未知 action fallback 两条路径同时断言。
- [x] 现状契约登记：`[C1.1]`、`[C2.1]`、`[C3.1]`、`[C3.2]`、`[C4.1]`、`[C4.2]`、`[C5.1]`、`[T1.1]`、`[T2.1]` 仅做对照不动代码，全部在表中标 pass + 现状契约。
- [x] 演练复跑：`make verify-fast` 全绿。
- [ ] 服务端 [C2.2] / [T1.2] 离线 surrendered 路径与 [D2.4] 一并由 A5 跟进。

## 4. 留底素材

- 代码改动集中在测试侧：`cmd/cli/state_apply_test.go`、`cmd/cli/table_screen_test.go`。本段没有实现层修复——cli 状态机此前已合规，缺的是回归绳子。
- 关键测试名（新增）：
  - `TestPlayerJourney_C1_2_ClaimDialogOnlyForCandidates`
  - `TestPlayerJourney_C3_3_ExplicitPassSendsPassRequest`
  - `TestPlayerJourney_T1_2_PassOnTsumoReturnsToDiscard`
  - `TestPlayerJourney_T2_2_TsumoDialogDefaultsToHu`
- `docs/spec/architecture-gaps.md` 新增 A17，状态「已锁定回归」。
