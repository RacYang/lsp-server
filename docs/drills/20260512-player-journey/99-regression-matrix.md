# 回归测试矩阵 · 20260512 player-journey 轮次

本表是本轮所有"根因修复 / 锁定契约"对应的 `TestPlayerJourney_*` 与既有测试名汇总，作为后续重构 / PR review 的快速核对入口。

## 1. 命名约定

- 新增测试一律使用 `TestPlayerJourney_<条款>_<场景>` 命名，与 `docs/spec/player-journey.md` v0.5 的条款编号一一对应。
- 既有测试若已经覆盖某条款但未带 spec 编号，本轮在 architecture-gaps.md 与 drill 文档里**附注其名**，不强行改名以避免无关 diff。
- 服务端 / 协议层缺口未在本轮落地的，在表里以 `deferred → <AID>` 标注。

## 2. 条款 ↔ 测试 ↔ AID 矩阵

| spec 条款 | 严重 | AID | 落地测试名 / 现状契约 | 触及文件 |
| --- | --- | --- | --- | --- |
| `[G11]` | P1 | A13 | `TestPlayerJourney_G11_NonOkLoginBlocks` | `silent_login*.go` |
| `[G12]` / `[G13]` | P0 | A1 | `TestPlayerJourney_G12_NoAutoPlayMark` + `TestPlayerJourney_G13_SurrenderRendersTriangle` | `scene.go` / `band.go` / `dialog_overlay.go` / `TILE-ART.md` |
| `[L2.3]` | P2 | A12 | `TestPlayerJourney_L2_3_LobbyHasNoProtocolIDs` | `scene_lobby.go` |
| `[L3.1]` / `[L3.2]` / `[G3]` | P0 | A2 | `TestPlayerJourney_L3_1_RoomAcceptsAutoMatchLocal` + `TestPlayerJourney_L3_2_AutoMatchFallsBackToCreateRoomLocal` + 既有 `TestRemoteRoomGatewayAutoMatchSkipsStartedRoom` | `gate_remote.go` / `local_gateway.go` |
| `[L5.2]` / `[P4.2]` | P0 | A3 | `TestPlayerJourney_L5_2_PrivateRoomCodeVisibleInPrep` + `TestPlayerJourney_P4_2_PrivateRoomCodePersistsUntilPlaying` | `scene.go::renderRoomPrep` / `lobby_types.go` / `ws_lobby_gateway.go` / `state*.go` |
| `[L10.1]` | P1 | A13 | `TestPlayerJourney_L10_1_RouteRedirectFlagsReconnecting` + 既有 `TestSilentLogin*` | `state_apply.go::Envelope_LoginResp` |
| `[E1.1]` | P1 | A14 | `TestPlayerJourney_E1_1_ExchangeThreeKeepsThirteenAuthoritativeTiles` | `state_apply.go` |
| `[E1.2]` | P1 | A14 | `TestPlayerJourney_E1_2_ExchangeMarkRejectsCrossSuit` | `table_screen.go::sameExchangeSuit` |
| `[E2.1]` | P1 | A14 | `TestCentralPromptStates`（更新断言为「已选 N/3」字面） | `interaction.go::primaryPrompt` |
| `[E2.2]` | P1 | A14 | `TestPlayerJourney_E2_2_ExchangeThreeRejectionSurfacesNotice` | `state_apply.go::Envelope_OpeningActionResp` |
| `[Q1.1]` | P1 | A15 | `TestPlayerJourney_Q1_1_QueMenKeysOnlyMPS` + `TestPlayerJourney_Q1_1_QueMenKeysSilentWhenNotAllowed` | `table_screen.go` |
| `[Q1.2]` | P1 | A15 | `TestPlayerJourney_Q1_2_OpeningMissingSuitDoneFillsRoster` | `state_apply.go::applyOpeningMissingSuitDone` |
| `[Q2.1]` | P2 | A15 | `TestCentralPromptStates`（hint 含「选定后不可更改」断言） | `interaction.go` |
| `[Q2.2]` | P1 | A16 | `TestPlayerJourney_Q2_2_QueSuitTileDecodingAndCursorOverlay` | `seat_tiles.go::drawSouthHand` |
| `[D1.1]` | P1 | A16 | `TestPlayerJourney_D1_1_PhaseDrawDoesNotWriteWaitingAction` | `state_apply.go` |
| `[D1.2]` | P1 | A16 | `TestPlayerJourney_D1_2_DrawTileBringsNewTileIntoSelfHand` + `TestPlayerJourney_D1_2_CursorLandsOnFreshlyDrawnTile` | `state_apply.go` / `discard_cursor.go::SyncMode` |
| `[D1.3]` | P1 | A16 | `TestPlayerJourney_D1_3_OtherSeatDrawHidesTile` | `state_apply.go::applyDraw` |
| `[D2.1]` | P1 | A16 | `TestPlayerJourney_D2_1_EnterSilentWhenNotMyTurn` | `table_screen.go::submitCursorAction` |
| `[D2.2]` | P1 | A16 | `TestPlayerJourney_D2_2_EnterDebouncedAfterSubmit` | `table_screen.go` |
| `[D2.4]` | P0 | A5 | deferred → A5（服务端 engine_timeout surrender） | `internal/service/room` |
| `[D5.1]` / `[D5.2]` | P1 | A8 | deferred → A8（杠四形态） | `internal/mahjong/*` / cli melds |
| `[D6.1]` / `[T3.1]` | P1 | A9 | deferred → A9（ScoringContext 上下文） | `internal/mahjong/fan` |
| `[D7.1]` | P0 | A4 | deferred → A4（tenpai 投影协议字段） | `api/proto/client/v1` |
| `[C1.1]` | MUST | 现状契约 | 既有 `interaction.go::claimActionsForSeat` 路径 + `TestPlayerJourney_C1_2_*` 间接覆盖 | `interaction.go` |
| `[C1.2]` | MUST | A17 | `TestPlayerJourney_C1_2_ClaimDialogOnlyForCandidates` | `interaction.go` / `state_apply.go` |
| `[C2.1]` | MUST | 现状契约 | 既有 `TestClaimDialogProgress*` | `dialog_claim.go` |
| `[C2.2]` | MUST | A5 | deferred → A5（服务端离线 surrender） | `internal/service/room` |
| `[C3.3]` | MUST | A17 | `TestPlayerJourney_C3_3_ExplicitPassSendsPassRequest`（含「未知 action」默认 Pass 兜底） | `table_screen.go::submitClaimAction` |
| `[T1.2]` | MUST | A17 | `TestPlayerJourney_T1_2_PassOnTsumoReturnsToDiscard` | `state_apply.go::Envelope_PassResp` |
| `[T2.2]` | SHOULD | A17 | `TestPlayerJourney_T2_2_TsumoDialogDefaultsToHu` | `dialog_claim.go::buildClaimDialog` |
| `[S2.2]` / `[S2.3]` | MUST | A10 | `TestPlayerJourney_S3_1_*` + `TestPlayerJourney_S4_1_*` 间接覆盖 fan_names 透传与 outcome 判定 | `main.go::snapshotSettlementSummary` |
| `[S3.1]` | MUST | A10 | `TestPlayerJourney_S3_1_MultiWinnerExpandsEachHuSeat` | `dialog_settlement.go` / `main.go` |
| `[S4.1]` | MUST | A10 | `TestPlayerJourney_S4_1_DrawPenaltiesAreSurfaced` | `dialog_settlement.go` / `main.go` |
| `[S5.1]` | MUST | A10 | `TestSettlementDialogIncludesAllScoresWhenRevealed`（断言三段键位文案） | `dialog_settlement.go` |
| `[S7.1]` / `[G14]` | MUST | A6 / A10 | `TestPlayerJourney_S7_1_SettlementZeroSum`（cli 侧护栏 + 服务端基线）；服务端 panic / 断言 deferred → A6 | `cmd/cli` + `internal/service/room` |
| `[R1.1]` | MUST | A18 | `TestPlayerJourney_R1_1_RestartIssuesLeaveThenAutoMatch` | `main.go::restartAfterSettlement` |
| `[R1.4]` | MUST | A19 | deferred → A19（多局衔接顶栏提示） | `state_apply.go::Envelope_StartGame` |
| `[R3.1]` | MUST | A20 | deferred → A20（离桌弃局 confirm） | `scene.go::handleSettleKey` |
| `[N1.2]` | MUST | A11 | `TestPlayerJourney_N1_2_SnapshotKeepsSelfSeatAndUserID` | `state_apply.go::applySnapshot` |
| `[N1.3]` | MUST | A11 | `TestPlayerJourney_N1_3_SnapshotStepDropsStaleAction` + 既有 `TestSnapshotStepDropsStaleDrawTile` | `state_apply.go::shouldDropStaleStep` |
| `[N2.1]` | MUST | A11 | `TestPlayerJourney_N2_1_OfflineEnterTriggersLeaveRoom` + 既有 `TestDrawNetOverlayOfflineShowsButton` | `table_screen.go` / `dialog_network.go` |
| `[N2.2]` | SHOULD | A11 | `TestPlayerJourney_N2_2_RouteRedirectFlagsReconnecting` + 既有 `TestApplyRouteRedirectMarksReconnecting` | `state_apply.go::Envelope_RouteRedirect` |
| `[N3.1]` | DELETED | — | 由 A1 的 G12/G13 覆盖 | `scene.go` / `band.go` |
| `[N4.1]` | MUST | A13 | 既有 `TestPlayerJourney_G11_NonOkLoginBlocks` + `TestSilentLogin*` | `silent_login.go` |

## 3. 验证记录

- 本轮根因修复全部使用 `make verify-fast` 校验：跑通 verify-fmt（gofmt + goimports）/ verify-lint（go vet + staticcheck via golangci-lint）/ verify-test-unit（cmd/... + internal/...）/ verify-git-local（路径名 / 二进制 / 体积卫生）/ markdownlint。
- 未触及 `.proto` 或派生 PB，故未触发完整 `make verify`；下次涉及 proto 的 PR（如 A4 / A5）必须 `make verify` 起步。
- `make verify-bench` 与 `RUN_INTEGRATION=1 make verify-test-integration-nodocker` 不在本轮范畴；服务端兜底（A5 / A6）落地时必须把这两条加进 PR 自检列表。

## 4. 候选工单（按优先级）

1. **A6**：服务端结算路径加 panic / 断言 `sum(seat_scores.total_fan) + sum(penalties.amount_signed) == 0`，并写入集成测试夹具。
2. **A5**：room engine_timeout 在抢答 / tsumo / discard 三类窗口的 surrender 收口，对应 `[C2.2]` / `[T1.2]` / `[D2.4]`。
3. **A4**：`SnapshotNotify` / `ActionNotify` 投影 tenpai 信息供 cli 显示听牌。
4. **A7**：`gate_remote` 路径 DrawTile per-seat 隐私等价回归。
5. **A8**：杠四形态在 `MeldInfo` 与 cli 渲染区分。
6. **A9**：海底 / 杠上花 / 杠上炮 `ScoringContext` 上下文 cli 展示。
7. **A19**：多局衔接「准备开始下一局」顶栏短提示。
8. **A20**：离桌弃局 confirm 弹窗（`[R3.1]` cli 提示部分）。
