# 房间预备段 · prep_segment

## 0. 上下文

- spec 节：§P 房间预备、§G 全局不变量
- drill 目录：`tmp/drills/20260512-prep/`（按需重新采集）
- 后端配置：`configs/dev.yaml`
- 涉及 AID：`A1`（cli 渲染托管态）、`A3`（私密房码持续展示）

## 1. 条款对照表

| 条款 | 等级 | 预期帧/事实 | 修复前现象 | 修复后断言 | 结论 | AID/测试名 |
| --- | --- | --- | --- | --- | --- | --- |
| `[G12]` | MUST | 帧文本中座位状态仅取自 `● ○ ▲ ✓ ▣ □` 六字符集合 | `cmd/cli/scene.go::seatPrepLabel` 与 `cmd/cli/band.go::seatStatusMark` 在 `p.AutoPlay=true` 时输出 `◐` | 预备页帧文本不含 `◐`，不含「托管」 | pass | `A1` / `TestPlayerJourney_G12_NoAutoPlayMark` |
| `[G13]` | MUST | 弃局态独立渲染为 `▲ 弃局`，与托管语义解耦 | `cmd/cli/dialog_overlay.go` 将 `p.Surrendered` 渲染为「托管中」 | 预备页含 `▲`，不含「托管」 | pass | `A1` / `TestPlayerJourney_G13_SurrenderRendersTriangle` |
| `[L5.2]` | MUST | 私密房创建后房间码作为分享凭据持续醒目展示 | `renderRoomPrep` 仅在第 3 行平铺一次 `房间: <room_id>`，私密无差异 | 私密房帧文本同时含房间码、`★`、「私密」字样并使用 `highlightStyle()` 高亮 | pass | `A3` / `TestPlayerJourney_L5_2_PrivateRoomCodeVisibleInPrep` |
| `[P4.2]` | MUST | 预备阶段 `waiting` 与 `ready` 子态房间码持续可见，直至进入 playing | 同上：state 切换无影响于渲染，但缺少回归断言 | `RoomState=waiting`/`ready` 两帧均含房间码 | pass | `A3` / `TestPlayerJourney_P4_2_PrivateRoomCodePersistsUntilPlaying` |

## 2. 关键发现

- 现象（A1）：服务端在玩家长离线触发 `surrender_after_offline` 时把 `SeatInfo.status=surrendered` 与 `SeatInfo.auto_play=true` 同时下发；旧 cli 既画 `◐` 又把 `Surrendered` 在浮窗显示为「托管中」，违反「托管 feature 暂不推进」的 v0.5 硬契约，并把「弃局」与「托管」的玩家心智混在一起。
- 现象（A3）：玩家走「3 创建房间 → 私密」三步后进入预备页，房间码仅出现在第 3 行普通正文位置，没有任何视觉强调；玩家无法在「等待好友输入房间码」的稳定状态下把码持续读出来。
- 根因（A1）：cli scene 与 dialog_overlay 在 `AutoPlay` / `Surrendered` 两个互不相关的字段上做了字符串合并，缺少「值域只能是 `● ○ ▲ ✓ ▣ □`」的硬约束。
- 根因（A3）：`client.v1.CreateRoomResponse` 不携带 `private` 字段，cli 长期没有把「我刚创建的是私密房」这条本地知识记账到 `RoomView`；renderRoomPrep 也没有为私密房分支预留高亮。
- 锚点条款：`[G12]`、`[G13]`、`[L5.2]`、`[P4.2]`
- AID：`A1`、`A3`

## 3. 修复跟踪

- [x] AID `A1`：
  - `cmd/cli/scene.go::seatPrepLabel` 删除 `case p.AutoPlay: mark = "◐"`，新增 `Hued → ✓`、`Surrendered → ▲` 分支并加注释引用 `[G12]`。
  - `cmd/cli/band.go::seatStatusMark` 同步切换值域，空座返回 `□`、`Hued/Surrendered` 优先于 `IsBot/Offline`。
  - `cmd/cli/dialog_overlay.go` 把 `p.Surrendered` 文案改为「▲ 弃局」并增加「✓ 已胡」分支。
  - `cmd/cli/TILE-ART.md` 徽章列表去掉「托管=⏸」一栏，改为「弃局/已胡」。
  - `RoomViewSeat.AutoPlay` 字段保留：reducer 仍写入、frame_log 仍上报，仅作为回归取证字段，不再驱动渲染。
- [x] AID `A3`：
  - `cmd/cli/lobby_types.go::LobbyJoinResult` 新增 `Private bool`。
  - `cmd/cli/ws_lobby_gateway.go::CreateRoom` 把 `opts.Private` 写入 `result.Private`（client.v1 协议不回包 private，发起方本地记账）。
  - `cmd/cli/ws_lobby_gateway.go::applyJoinResultToState` 透传到 `RoomView.Private`，AutoMatch/JoinRoom 路径写 false，避免私密标签泄漏到下一房。
  - `cmd/cli/state.go::RoomView` 新增 `Private bool`；`state_apply.go::resetRoomToLobby` 复位为 false。
  - `cmd/cli/scene.go::renderRoomPrep` 在 `view.Private` 时把房间码加 `★ 私密房间码（分享给好友加入）：` 前缀并使用 `highlightStyle()`（黄色加粗）。
  - `cmd/cli/table_render.go` 新增 `highlightStyle()`，仅切前景色与 Bold，避免与其他渲染抢占列宽。
- [x] 回归断言：上表四条 `TestPlayerJourney_*` 全部新增并通过；既有 `TestSceneRouterRenderRoomPrep` 等 golden 用例不受影响。
- [x] 演练复跑：`make verify-fast` 全绿。
- [ ] 真实端到端复跑：`LSP_JOURNEY_DRIVE=1 go test -run TestPlayerJourneyAgainstRealBackend` 与 `scripts/drills-dev.sh start --cli` 手测预留给下一段 `exchange_que_segment` 之前由人工触发。

## 4. 留底素材

- 修复 commit 范围：本段所有改动集中在 `cmd/cli/{scene,band,dialog_overlay,state,state_apply,table_render,lobby_types,ws_lobby_gateway}.go` + `cmd/cli/TILE-ART.md` + `cmd/cli/scene_test.go`。
- 关键测试名：`TestPlayerJourney_G12_NoAutoPlayMark`、`TestPlayerJourney_G13_SurrenderRendersTriangle`、`TestPlayerJourney_L5_2_PrivateRoomCodeVisibleInPrep`、`TestPlayerJourney_P4_2_PrivateRoomCodePersistsUntilPlaying`。
- 与 `docs/spec/architecture-gaps.md` 的 A1 / A3 状态同步标记「已修复」。
