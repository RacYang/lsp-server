# 断线 + 重连段 · reconnect_segment

## 0. 上下文

- spec 节：§N 断线与重连
- 涉及 AID：`A11`（断线浮窗 + 长离线段 cli 回归补齐）；关联 `A5`（服务端 `surrender_after_offline` 兜底）、`A13`（LoginResp 非 OK 阻断）
- 协议字段：`SnapshotNotify.{last_step,player_ids,your_hand_tiles}` / `RouteRedirectNotify.ws_url` / `LoginResponse.error_code` / `LeaveRoomReq`

## 1. 条款对照表

| 条款 | 等级 | 预期帧/事实 | 现状 | 修复后断言 | 结论 | AID/测试名 |
| --- | --- | --- | --- | --- | --- | --- |
| `[N1.1]` | MUST | 短暂断线顶栏出现 `○ 重连中`，不清场景 | `state_apply.go` 在断线 / RouteRedirect 时置 `v.Reconnecting=true`；`networkLabel(view)` 在顶栏渲染；`dialog_network.go` 同时维护 `NetOverlayState` 的 `NetStatusReconnecting`；既有 `TestNetOverlayEnterReconnectingOnDisconnect` 覆盖 | 既有 net overlay 单元 + 顶栏 `Reconnecting` 字段 | pass | 既有测试 |
| `[N1.2]` | MUST | 重连后以 `SnapshotRoom` / `SnapshotNotify` 为权威恢复座位 / 用户；不漂移 | `applySnapshot` 写 RoomID / RoundPhase / YourHandTiles / SeatRoster；本家 `SeatIndex` 来自更早 `JoinRoomResp`，快照写 `PlayerIds[seat].UserID` 与之对齐 | `TestPlayerJourney_N1_2_SnapshotKeepsSelfSeatAndUserID` 钉住 SeatIndex/UserID/Players[SeatIndex].UserID 三方一致 | locked | `A11` / 同名测试 |
| `[N1.3]` | MUST | 重连按 `SnapshotNotify.last_step` 切点丢弃陈旧增量 | `shouldDropStaleStep` 在 `state.Apply` 入口判断 `step>0 && SnapshotStep>0 && step<=SnapshotStep`，命中即写日志直接 return | `TestPlayerJourney_N1_3_SnapshotStepDropsStaleAction` 钉住 ActionNotify 旧 step 不污染手牌；既有 `TestSnapshotStepDropsStaleDrawTile` 覆盖 DrawTile 路径 | locked | `A11` / 同名测试 |
| `[N2.1]` | MUST | 长离线浮窗增加「返回大厅」按钮且可用，按下走 `LeaveRoom` + 回大厅，并按 `[R3.1]` 提示弃局影响 | `NetOverlayState` 在 `now-Since >= Threshold` 时升 `NetStatusOffline`；`DrawNetOverlay` 渲染 `[ 返回大厅 ]`；`handleTableKey` 在 Offline + KeyEnter 时显式 `gateway.LeaveRoom` + `TableExitLeaveRoom`。`[R3.1]` 离桌弃局 confirm 仍在 `A20` 候选 | `TestPlayerJourney_N2_1_OfflineEnterTriggersLeaveRoom` 钉住 Offline+Enter→LeaveRoom 路径；既有 `TestDrawNetOverlayOfflineShowsButton` 覆盖按钮渲染 | locked | `A11` / 同名测试，`[R3.1]` 提示 → A20 |
| `[N2.2]` | SHOULD | `RouteRedirectNotify` 必须在顶栏显著提示「服务端切换网关」并自动重连 | `state_apply.go::Envelope_RouteRedirect` 写 `Reconnecting=true` + `LastError="服务端要求切换网关"` + appendLog；网络层根据 `ws_url` 自动重连 | `TestPlayerJourney_N2_2_RouteRedirectFlagsReconnecting` 钉住 Reconnecting 与 LastError；既有 `TestApplyRouteRedirectMarksReconnecting` 覆盖 reducer | locked | `A11` / 同名测试 |
| `[N3.1]` | DELETED | 原"托管恢复"条款；spec 0.5 已删除，由 `[G12]/[G13]` + `surrender_after_offline` 覆盖 | cli 不渲染托管态，长离线直接判弃局，重连后显 `▲ 弃局` | 现状契约（A1 已锁定 G12/G13 渲染） | pass | A1 |
| `[N4.1]` | MUST | 服务端版本不兼容（重连 LoginResp 非 OK）必须走 `[L10.1]` 阻断路径而非反复重连 | `applyLoginResp` 把非 OK 的 LoginResponse 落入 `BlockingError` 并阻塞重连循环；既有 `TestPlayerJourney_G11_NonOkLoginBlocks` 覆盖 | 现状契约（A13 已锁定） | pass | A13 |

## 2. 关键发现

- §N 在 cli 这一侧的状态机长期已经按 `Reconnecting` / `SnapshotStep` / `NetStatusOffline` 三条独立信号驱动，但**这些不变量从未被打上 spec 编号**。本段主要工作是把它们逐条钉死：未来重构 reducer / 网络层时，破坏 `[N1.2]` 或 `[N1.3]` 的提交会立刻在 cli 包内红，比"等到联调出 bug"早 N 天。
- `[N1.3]` 的 stale-drop 通道目前覆盖 `InitialDeal / DrawTile / Action / OpeningDone / StartGame` 五类增量（见 `envelopeStep`）。`Settlement / RouteRedirect / Snapshot` 自身不走 step 切点（前者属"窗口结束"事件、后者本身就是权威源）。如果未来加新的 step-bearing notify，应当**同步扩展 `envelopeStep`**——这是一处典型的"加字段不扩 switch 就会漏丢弃"陷阱，建议下次扩展时先补 N1.3 用例再加字段。
- `[N2.1]` 离桌时**目前没有 confirm 弹窗**。`runtime.room.allow_leave_during_play=true` 时离桌会被服务端判弃局，玩家可能因为误按 Enter 而失分；`[R3.1]` 已经把"必须显式提示"列为 MUST，但 cli 这一侧仍在 `A20` 候选。本段先把 Offline + Enter 的 RPC 链路钉死，confirm 提示作为下一轮 UX 单独提。
- `[N4.1]` 与 `[L10.1]` 共用 `BlockingError` 路径（A13 已修复），本段不重复回归。但要注意：**重连过程**中收到非 OK LoginResp 时也必须走同一路径，而不是仅在首登失败时阻断；现状 `silent_login.go` 已经把两条路径合并到同一 reducer，回归用例分布在 `TestPlayerJourney_G11_*` 与 `TestSilentLogin*`。

## 3. 修复跟踪

- [x] 增 `TestPlayerJourney_N1_2_SnapshotKeepsSelfSeatAndUserID`
- [x] 增 `TestPlayerJourney_N1_3_SnapshotStepDropsStaleAction`
- [x] 增 `TestPlayerJourney_N2_1_OfflineEnterTriggersLeaveRoom`
- [x] 增 `TestPlayerJourney_N2_2_RouteRedirectFlagsReconnecting`
- [x] `make verify-fast` 全绿
- [ ] `[N2.1]` / `[R3.1]` 离桌弃局 confirm 提示：候选 `A20`
- [ ] 服务端 `surrender_after_offline` 兜底回归：`A5`
