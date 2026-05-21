# 玩家旅程 spec 对照现状：架构缺陷候选清单 v0.1

本文档用 [docs/spec/player-journey.md](player-journey.md) v0.5 的每条 MUST 扫一遍当前实现，列出"现状无法承载 spec"的 gap。**仅作纸面回放**，不在本步动代码；每条 gap 标注：

- **条款**：spec 编号
- **现状**：定位到具体文件 / 行 / 字段
- **gap 性质**：协议层 / 服务端 / 网关 / cli reducer / cli scene / runtime / 可观测
- **修复责任**：哪个模块拥有根因，避免"哪里疼贴哪里"
- **严重程度**：P0（玩家立刻看到错） / P1（边界 / 隐私 / 一致性） / P2（体感 / 文案）
- **回归测试名**：以 `TestPlayerJourney_<条款>_<场景>` 命名，绑定本 spec 条款

未列入本文档的 MUST 条款，视为"现状已基本承载，待 drills 复核"，不代表正确无需测试，只代表本轮不立硬规则。

---

## 0. 摘要

| ID  | 严重 | 条款                          | 一句话                                                               |
| --- | ---- | ----------------------------- | -------------------------------------------------------------------- |
| A1  | P0   | `[G12]`/`[G13]`/`[N3.1]`      | cli 仍把 `SeatInfo.auto_play` 渲染为 `◐` 与"托管中"文案。**状态：已修复**（`TestPlayerJourney_G12_NoAutoPlayMark` + `TestPlayerJourney_G13_SurrenderRendersTriangle`） |
| A2  | P0   | `[L3.1]`/`[G3]`               | AutoMatch 探活 stage 跳过 playing 的逻辑在两条路径对齐。**状态：已修复**（`TestPlayerJourney_L3_1_RoomAcceptsAutoMatchLocal` + `TestPlayerJourney_L3_2_AutoMatchFallsBackToCreateRoomLocal` + 既有 `TestRemoteRoomGatewayAutoMatchSkipsStartedRoom`） |
| A3  | P0   | `[L5.2]`/`[P4.2]`             | 私密房 `room_id` 在预备页顶栏没有持续醒目展示。**状态：已修复**（`TestPlayerJourney_L5_2_PrivateRoomCodeVisibleInPrep` + `TestPlayerJourney_P4_2_PrivateRoomCodePersistsUntilPlaying`） |
| A4  | P0   | `[D7.1]`                      | `client.v1` 协议无 tenpai 投影字段，cli 无法显示听牌                 |
| A5  | P0   | `[D2.4]`/`[C2.2]`/`[C3.3]`/`[T1.2]`/`[G12]` | 服务端超时路径需明确"surrender / 显式 pass"语义，不得保留任何"代打具体牌"分支 |
| A6  | P1   | `[G14]`/`[S7.1]`              | 结算零和无显式断言，仅"以权威值为准"模糊处理                         |
| A7  | P1   | `[D1.3]`/`[G9]`               | DrawTile per-seat 隐私在 `local_gateway` 有测试覆盖，`gate_remote` 路径需补等价测试 |
| A8  | P1   | `[D5.1]`/`[D5.2]`             | 杠形态在 `MeldInfo` 与 cli 副露轨道中区分。**状态：已修复**（`TestApplyAnGangRecordsConcealedMeldAndHidesActionTile` + `TestApplyBuGangCompletesWithoutRobCandidate` + `TestApplyBuGangCanBeRobbedWithoutGangRecord` + `TestFormatMeldGlyphsDistinguishesMeldKinds`） |
| A9  | P1   | `[D6.1]`/`[T3.1]`             | 海底 / 杠上花 / 杠上炮等 `ScoringContext` 上下文在 cli 是否展示待补    |
| A10 | P1   | `[S3.1]`/`[S4.1]`/`[S5.1]`    | 多家胡只保留第一个 winner 显示；流局 penalties 未在浮窗 / stdout 暴露；底栏键位文案错（"R 再来一局/L 离桌"）。**状态：已修复**（`TestPlayerJourney_S3_1_MultiWinnerExpandsEachHuSeat` + `TestPlayerJourney_S4_1_DrawPenaltiesAreSurfaced` + `TestSettlementDialogIncludesAllScoresWhenRevealed` 更新）。`[S7.1]` 零和的服务端断言归 A6 跟进；本段已加 `TestPlayerJourney_S7_1_SettlementZeroSum` 作为 cli 侧护栏 + 服务端基线。 |
| A18 | P0   | `[R1.1]`/`[R1.2]`             | 再开一桌 LeaveRoom→AutoMatch 顺序无回归；既有 `restartAfterSettlement` 在 LeaveRoom 失败时仍继续 AutoMatch（与 [R1.1] 严格 reading "真请求成功" 存在偏差，目前保留并以注释 + 测试锁定意图）。**状态：已锁定回归**（`TestPlayerJourney_R1_1_RestartIssuesLeaveThenAutoMatch`）。 |
| A11 | P1   | `[N1.2]`/`[N1.3]`/`[N2.1]`/`[N2.2]` | 重连快照不漂移座位 / 用户、按 `last_step` 切点丢弃陈旧增量、长离线 Enter→LeaveRoom 兜底、`RouteRedirect` 顶栏提示均缺独立 spec-tagged 回归。**状态：已锁定回归**（`TestPlayerJourney_N1_2_SnapshotKeepsSelfSeatAndUserID` + `TestPlayerJourney_N1_3_SnapshotStepDropsStaleAction` + `TestPlayerJourney_N2_1_OfflineEnterTriggersLeaveRoom` + `TestPlayerJourney_N2_2_RouteRedirectFlagsReconnecting`）。`[N1.1]` 顶栏「○ 重连中」沿用既有 `Reconnecting` 字段，文案调整属 A12 / 后续 UX 工单。 |
| A12 | P2   | `[L2.3]`                      | 大厅 UI 中协议 ID 隔离。**状态：已修复**（`TestPlayerJourney_L2_3_LobbyHasNoProtocolIDs`） |
| A13 | P2   | `[G11]`/`[L10.1]`             | LoginResp 非 OK 阻断与路由重定向路径。**状态：已修复**（`TestPlayerJourney_G11_NonOkLoginBlocks` + `TestPlayerJourney_L10_1_RouteRedirectFlagsReconnecting`） |
| A14 | P1   | `[E1.2]`/`[E2.1]`/`[E2.2]`    | 换三张 UI 层未拒绝异花色标记；提示文案未用「已选 N/3」字面；服务端拒绝未落 UXTransient。**状态：已修复**（`TestPlayerJourney_E1_1_*` + `TestPlayerJourney_E1_2_*` + `TestPlayerJourney_E2_2_*` + `TestCentralPromptStates` 更新） |
| A15 | P1   | `[Q1.1]`/`[Q2.1]`             | 定缺接受 1/2/3 数字快捷键并对非 que_men 阶段的 m/p/s 产生 UXTransient 副作用；提示文案未告知「选定后不可更改」。**状态：已修复**（`TestPlayerJourney_Q1_1_QueMenKeysOnlyMPS` + `TestPlayerJourney_Q1_1_QueMenKeysSilentWhenNotAllowed` + `TestPlayerJourney_Q1_2_OpeningMissingSuitDoneFillsRoster`） |
| A16 | P1   | `[D1.2]`/`[D2.1]`/`[Q2.2]`    | DrawTile 后光标固定 handLen-1 而非新摸牌位置；非本家回合 Enter 弹「当前不能操作手牌」副作用；自家手牌缺门花色未灰显。**状态：已修复**（`TestPlayerJourney_D1_1_*` + `TestPlayerJourney_D1_2_*` + `TestPlayerJourney_D1_3_*` + `TestPlayerJourney_D2_1_EnterSilentWhenNotMyTurn` + `TestPlayerJourney_D2_2_EnterDebouncedAfterSubmit` + `TestPlayerJourney_Q2_2_QueSuitTileDecodingAndCursorOverlay`） |
| A17 | P2   | `[C1.2]`/`[C3.3]`/`[T1.2]`/`[T2.2]` | 抢答 / 自摸窗口的候选裁决、显式 pass 路由、不胡后回 discard、tsumo 默认高亮等行为缺独立回归。**状态：已锁定回归**（`TestPlayerJourney_C1_2_ClaimDialogOnlyForCandidates` + `TestPlayerJourney_C3_3_ExplicitPassSendsPassRequest` + `TestPlayerJourney_T1_2_PassOnTsumoReturnsToDiscard` + `TestPlayerJourney_T2_2_TsumoDialogDefaultsToHu`）。`[C2.2]` 离线 → surrendered 与 `[T1.2]` 离线 PassRequest 不发的服务端兜底属于 A5 跟进。 |

---

## 1. cli 层 gap

### A1 (P0) cli 仍渲染托管态，违反 `[G12]` / `[G13]` / `[N3.1]`

**状态：已修复**（`TestPlayerJourney_G12_NoAutoPlayMark` + `TestPlayerJourney_G13_SurrenderRendersTriangle`）。`cmd/cli/scene.go::seatPrepLabel` 与 `cmd/cli/band.go::seatStatusMark` 移除 `p.AutoPlay → ◐` 分支，新增 `Hued → ✓` 与 `Surrendered → ▲` 分支；`cmd/cli/dialog_overlay.go` 把"托管中"文案改为"▲ 弃局"+"✓ 已胡"。`SeatInfo.auto_play` 仍在 reducer 里保留并被 frame_log 上报，仅作为回归断言的取证字段，不参与渲染。

**现状**：

- `cmd/cli/state.go:88-90` `RoomViewSeat.AutoPlay bool` 字段存在。
- `cmd/cli/state_apply.go:650-651` 直接把服务端 `SeatInfo.GetAutoPlay()` 写入本地 `RoomView`。
- `cmd/cli/scene.go:331-333` 与 `cmd/cli/band.go:208-210` 按 `p.AutoPlay` 渲染 `◐` 图标。
- `cmd/cli/dialog_overlay.go:171-172` 在 `p.Surrendered` 分支输出 `"  托管中"` 文案——并且把"弃局"和"托管"混淆。
- `cmd/cli/TILE-ART.md:27-28` 字符集列表里仍把"托管"作为标准图标项。

**gap 性质**：cli scene + cli reducer + 文档。**与服务端协议无关**：spec `[G12]` 明确"客户端不渲染 auto_play"，即便服务端字段长期保留也由 cli 自降级。

**修复责任**：cli。即便协议长期演进决定是否删字段，cli 这一轮就要把"托管"图标与文案下线，`Surrendered` 渲染独立的 `▲ 弃局`，不蹭"托管中"。

**回归测试名**：

- `TestPlayerJourney_G12_NoAutoPlayMark`：构造 `SeatInfo.auto_play=true` 的快照投到 cli，断言任一帧文本不包含 `◐` 与 `"托管"` 子串。
- `TestPlayerJourney_G13_SurrenderRendersTriangle`：达 `surrender_after_offline` 后 `SeatInfo.status=surrendered`，断言帧文本含 `▲ 弃局` 且不含"托管"。

---

### A3 (P0) 私密房创建后房间码不持续展示，违反 `[L5.2]` / `[P4.2]`

**状态：已修复**（`TestPlayerJourney_L5_2_PrivateRoomCodeVisibleInPrep` + `TestPlayerJourney_P4_2_PrivateRoomCodePersistsUntilPlaying`）。`LobbyJoinResult` 新增 `Private bool`，由 `ws_lobby_gateway.go::CreateRoom` 在调用方本地写入（client.v1 `CreateRoomResponse` 没有回包 private 字段，发起方自记账）；`applyJoinResultToState` 透传到 `RoomView.Private`，`resetRoomToLobby` 复位为 false；`renderRoomPrep` 在 `view.Private` 时把房间码加 ★ 前缀并用黄色加粗 `highlightStyle()` 持续展示，进入 playing 后 cli 自然切到 round 场景，prep 渲染分支停止生效。

**现状**：

- `cmd/cli/scene_lobby.go` 的创建向导路径在三步完成后直接 `JoinRoom` + 切到 `SceneRoomPrep`，没有"把 `CreateRoomResponse.room_id` 在顶栏作为房间码持续展示"的逻辑。
- 现 cli 仅有"显示 `room_id` 一闪即过"的提示，没有按 spec `[L5.2]` 要求"持续展示直至进入 playing"。

**gap 性质**：cli scene（`SceneRoomPrep` 顶栏渲染）。协议侧 `CreateRoomResponse.room_id` 已有；`SnapshotRoom.RoomMeta` 也含 `room_id`，**协议字段够用**。

**修复责任**：cli。在 `SceneRoomPrep` 顶栏增加"私密房"分支：`view.RoomMeta.private=true` 时把 `room_id` 持续高亮显示直到 `RoomLifecycle=playing`。

**回归测试名**：

- `TestPlayerJourney_L5_2_PrivateRoomCodeVisibleInPrep`：模拟创建私密房后切到 `SceneRoomPrep`，断言连续 N 帧顶栏文本均包含 `room_id` 子串。
- `TestPlayerJourney_P4_2_PrivateRoomCodePersistsUntilPlaying`：FSM 从 waiting → ready → playing，断言 `waiting` 与 `ready` 阶段每帧顶栏含 room_id，进入 playing 后允许收起。

---

### A2 (P0) AutoMatch 探活仅在 `local_gateway` 落地，`gate_remote` 路径需对齐，违反 `[L3.1]` / `[G3]`

**现状**：

- 用户当前未提交的 diff 显示 `internal/app/gate_remote.go` 与 `internal/handler/local_gateway.go` 都在改 AutoMatch；`internal/app/gate_remote_pure_test.go` 已新增对应用例。
- 此前回归用例（`cmd/cli/scene_round_e2e_test.go` 等）只覆盖单进程聚合路径；spec `[G3]` 要求两条 gateway 字段集等价、行为等价。

**gap 性质**：网关层（`internal/handler/local_gateway.go` vs `internal/app/gate_remote.go`），属于 ADR-0044 决策 1 的事实源等价契约。

**修复责任**：网关层。两条路径必须共享同一份"跳过 `state ∈ {playing, settling, closed}` 的房 + 找不到则 `CreateRoom`"实现，最好抽取共通 helper 给两侧调用。

**回归测试名**：

- `TestPlayerJourney_L3_1_AutoMatchSkipsPlayingRoomLocal`
- `TestPlayerJourney_L3_1_AutoMatchSkipsPlayingRoomRemote`
- `TestPlayerJourney_L3_2_AutoMatchFallsBackToCreateRoom`：两条路径均断言无空房时回落到 `CreateRoom`，且没有把玩家塞进 `state=playing` 的房。

---

### A12 (P2) 大厅 UI 是否泄漏协议 ID，违反 `[L2.3]`

**现状**：尚未做全局扫描；`cmd/cli/scene_lobby.go` / `cmd/cli/lobby_types.go` 含 `RoomMeta.stage / rule_id / page_token` 字段流转，能否保证不"原样显示"待复核。

**gap 性质**：cli scene 文案层。

**回归测试名**：

- `TestPlayerJourney_L2_3_LobbyHasNoProtocolIDs`：grep 帧文本不含 `rule_id`、`page_token`、`req_id` 等子串。

---

### A13 (P2) LoginResp 非 OK 阻断与版本不兼容路径，违反 `[G11]` / `[L10.1]`

**现状**：`cmd/cli/silent_login.go` 与 `cmd/cli/main.go` 处理 LoginResp 的失败路径需补"显式阻断 + 可读提示"帧文本断言。

**回归测试名**：

- `TestPlayerJourney_G11_NonOkLoginBlocks`
- `TestPlayerJourney_L10_1_VersionMismatchBlocks`

---

## 2. 协议层 gap

### A4 (P0) `client.v1` 无 tenpai 投影字段，违反 `[D7.1]`

**现状**：

- 服务端已具备听牌算法：`internal/mahjong/hu/ting.go`、`internal/mahjong/analysis/analysis.go`。
- `api/proto/client/v1/messages.proto` 无 `tenpai_by_seat` 等字段；client.v1 端无承载。
- 因此 TUI 无法按 `[D7.1]` 显式提示听牌；玩家失去"自己叫不叫"的核心反馈。

**gap 性质**：协议层。需要新增字段 + projector 投影 + cli 渲染三段联动。

**修复责任**：协议侧 `add-pb-message` skill 路径走完整流程；不在 cli 本地推断，避免与本家手牌 / 隐私边界冲突。

**回归测试名**：

- `TestPlayerJourney_D7_1_TenpaiBackendEmits`：服务端 round projector 检测到本家叫时，下发事件含 tenpai 字段。
- `TestPlayerJourney_D7_1_TenpaiVisibleInTUI`：cli 帧文本含可读"听牌"提示。

---

## 3. 服务端 round / engine gap

### A5 (P0) 超时路径必须收口为"surrender / 显式 pass"，违反 `[D2.4]` / `[C2.2]` / `[C3.3]` / `[T1.2]` / `[G12]`

**现状**：

- 出牌超时：`internal/service/room/engine_timeout.go` 已有 surrender 入口（grep 命中），需复核所有 `available_actions` 含 `discard` 但玩家未发出动作的窗口是否都走 surrender。
- 抢答 / 自摸窗口：spec 要求"客户端发 `PassRequest`"；服务端兜底也只能 `pass` 或对离线玩家直接 surrender。是否存在"服务端兜底自动碰 / 杠 / 胡"路径需扫 `engine_*.go` 全文件。
- bot supervisor：`runtime.lobby.bot_supervisor_enabled` 通过的"机器人占座"不是托管，但与"代打"边界容易混淆——这两套必须在代码里有明确隔离。

**gap 性质**：服务端 room service。属于 ADR-0044 决策 8（哪些动作必须显式 ack）的硬性边界。

**修复责任**：服务端。本轮不实现"完整托管"，所有超时分支显式走 surrender 或 pass。

**回归测试名**：

- `TestPlayerJourney_D2_4_DiscardTimeoutSurrenders`
- `TestPlayerJourney_C2_2_ClaimTimeoutPassesOrSurrenders`
- `TestPlayerJourney_T1_2_TsumoTimeoutPassesOrSurrenders`
- `TestPlayerJourney_G12_NoServerSideAutoPlay`：玩家未发任何动作的连续 N 个超时窗口里，没有 `ActionNotify(source=player, kind ∈ {discard, peng, gang, hu})` 来自该 user_id。

---

### A6 (P1) 结算零和无显式断言，违反 `[G14]` / `[S7.1]`

**现状**：

- `internal/mahjong/sichuan/xuezhandaodi/settlement.go` 与同目录 `settlement_test.go` 计算 `seat_scores` 与 `penalties`，但回归测试未对"零和"做显式 sum == 0 断言；线上 cli 端是"显示权威值"路径，对零和违例无任何告警。
- spec `[G14]` 升为 MUST：违例直接 fail，并 dump 字段。

**gap 性质**：服务端结算 + 回归测试。

**修复责任**：服务端结算回归。具体动作：在 `settlement_test.go` 增加零和断言；在 `internal/service/room` 派发结算前增加 defensive assert，违例直接 panic + structured log。

**回归测试名**：

- `TestPlayerJourney_G14_SettlementZeroSumStrict`
- `TestPlayerJourney_S3_1_MultiWinnersZeroSum`：一炮多响 / 血战连续胡场景下零和。
- `TestPlayerJourney_S4_1_DrawSettlementZeroSum`：流局查叫 / 花猪赔付 / 退税总和为 0。

---

### A8 (P1) 杠形态 / 暗杠隐私，违反 `[D5.1]` / `[D5.2]`

**现状**：

- `MeldInfo.kind` 已区分 `zhi_gang` / `an_gang` / `bu_gang`，TUI 副露轨道按 `直杠` / `暗杠` / `补杠` 展示。
- 暗杠动作通知使用 per-seat 投影：非本人 `ActionNotify.tile` 与 `detail.tile` 为空，cli 将未知暗杠牌面渲染为"暗牌 暗杠"。

**gap 性质**：已修复；后续若新增完整快照 per-seat 结构化副露过滤，需复用同一隐私规则。

**回归测试名**：

- `TestApplyAnGangRecordsConcealedMeldAndHidesActionTile`
- `TestApplyBuGangCompletesWithoutRobCandidate`
- `TestApplyBuGangCanBeRobbedWithoutGangRecord`
- `TestFormatMeldGlyphsDistinguishesMeldKinds`

---

### A9 (P1) 海底 / 杠上花 / 杠上炮等 `ScoringContext` 在 cli 展示，违反 `[D6.1]` / `[T3.1]`

**现状**：spec 要求服务端 `ScoringContext` 标记，cli 展示但不推断。需扫 `internal/mahjong/sichuan/xuezhandaodi` 与 cli 结算渲染。

**回归测试名**：

- `TestPlayerJourney_D6_1_ScoringContextLabeled`
- `TestPlayerJourney_T3_1_TsumoContextVisible`

---

### A10 (P1) 一炮多响 / 流局赔付的多家拆分，违反 `[S3.1]` / `[S4.1]`

**现状**：cli 结算浮窗目前是否"只显示第一个胡者"待复核；spec 要求每家拆分独立显示。

**回归测试名**：

- `TestPlayerJourney_S3_1_MultiWinnersShownSeparately`
- `TestPlayerJourney_S4_1_DrawPenaltiesShown`

---

## 4. 网关层 gap

### A7 (P1) DrawTile per-seat 隐私在 `gate_remote` 路径等价回归，违反 `[G3]` / `[D1.3]` / `[G9]`

**现状**：

- `internal/handler/local_gateway_test.go:89-101` 显示 LocalGateway 把非本家的 `DrawTileNotify.Tile` 抹为 `""`。
- `internal/app/gate_remote.go` 的对应路径需要同等的 per-seat 透视投影测试——spec `[G3]` 要求两条 gateway 字段集合等价。

**gap 性质**：网关层。

**回归测试名**：

- `TestPlayerJourney_D1_3_DrawTilePerSeatPrivacy_Remote`
- `TestPlayerJourney_G3_LocalAndRemoteFieldsEquivalent`：同一 trace_id 下两条路径事件字段集合相等。

---

## 5. 客户端 reducer gap（与 ADR-0044 决策 2/3 联动）

### 5.1 `[D1.1]` 已基本承载，但需补回归

**现状**：

- `cmd/cli/state_apply.go:142-144`、`:495-499` 显示 `applyRoundProgress` 是写 `WaitingAction` 的唯一入口；摸牌路径走 `DrawTileNotify` 应当**不**进入 `applyRoundProgress` 的 `WaitingAction=draw` 分支。
- 此前 commit `9736178 fix(room): 冻结摸牌通知投影状态` 已修过该回归；spec 要求继续验证。

**回归测试名**：

- `TestPlayerJourney_D1_1_DrawNotifyDoesNotWriteWaitingAction`：构造 DrawTileNotify 序列，断言 `view.WaitingAction != "draw"`。

---

### A11 (P1) 重连按 `last_step` 切点丢弃陈旧增量，违反 `[N1.3]`

**现状**：spec 要求按 `SnapshotNotify.last_step` 切点丢弃陈旧增量（ADR-0039 决策 3）。`cmd/cli/state_apply.go` 与 reconnect 路径是否有 last_step 比较待复核。

**回归测试名**：

- `TestPlayerJourney_N1_3_ReconnectDiscardsStaleDelta`

---

## 6. runtime / 可观测 gap

本轮未发现 runtime 参数缺位（`surrender_after_offline`、`surrender_action_timeout`、`allow_leave_during_play`、`bot_supervisor_enabled` 均在 `internal/config/config.go` 与 `configs/dev.yaml` 落地，与 spec 0.1 节列出值一致）。

可观测侧需要 drills 阶段补：

- 每次帧异常时 dump `RoundProgress` / `SeatRoster` 子集（与 `LSP_FRAME_LOG` 钩子联动），便于把 spec 条款 → 实际帧 → 后端日志切片 三段对齐。
- 不在本文档定义，留给 drills 任务 `scope_microscope`。

---

## 7. 与 ADR / 现有计划的联动

- ADR-0044 决策 7（cli 不本地写 ready / bot）被 A1（cli 渲染托管）间接挑战，但根因是 cli 渲染，**不是**本地写状态；不需改 ADR。
- ADR-0014 决策 5、ADR-0018 已明确 `surrender_after_offline` 在 gate 层调度，A5 不需要改 ADR，只需收口 room engine 超时分支。
- ADR-0039 决策 2/3/4 与 A4、A7、A8 联动；任何 proto 演进（特别是 A4 tenpai 字段）须按 `add-pb-message` skill 走完整流程并更新 ADR-0044 五类事实表。
- `lobby_segment` / `prep_segment` 等子段任务直接消费本文档：每个段的"修"动作需绑定到对应的 AID。

---

## 8. 下一步

1. 解冻 player-journey 后这份文档作为"接下来 drills + 测试"的活清单。
2. 进入 `scope_microscope` 任务时优先把 A1 / A2 / A3 / A4 / A5 / A6 五个 P0 + 一个 P1 串成第一轮 drill 的剧本。
3. 任何回归测试新增都必须以本文档中的 `TestPlayerJourney_<条款>_<场景>` 命名，确保"修复 → 条款 → 测试"三段始终可追溯。
4. 本文档随实施进展更新；AID 不重号，已修复条目改为 `状态: 已修复 + 测试名` 而不是删除，留作历史可追溯。
