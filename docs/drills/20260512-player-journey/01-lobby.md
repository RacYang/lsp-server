# L 大厅 · 20260512

## 0. 上下文

- spec 节：§1（大厅 L1..L10）
- 关联 AID：[A2](../../spec/architecture-gaps.md#a2)、[A12](../../spec/architecture-gaps.md#a12)、[A13](../../spec/architecture-gaps.md#a13)
- 实现复核范围：[internal/handler/local_gateway.go](../../../internal/handler/local_gateway.go)、[internal/app/gate_remote.go](../../../internal/app/gate_remote.go)、[cmd/cli/scene_lobby.go](../../../cmd/cli/scene_lobby.go)、[cmd/cli/state_apply.go](../../../cmd/cli/state_apply.go)

## 1. 条款对照表

| 条款 | 等级 | 预期 | 现状证据 | 结论 | AID/测试名 |
| --- | --- | --- | --- | --- | --- |
| `[L1.1]` | MUST | 启动后若 `~/.lsp/config.toml` 存在 `SessionToken` 必须走静默续登 | [cmd/cli/silent_login.go](../../../cmd/cli/silent_login.go) 与 `cmd/cli/silent_login_test.go` 覆盖 | pass | 既有 silent_login 单测覆盖 |
| `[L1.2]` | MUST | 续登失败 / token 过期 / 网络不可达必须落回登录页要求重输 | [applyLogin](../../../cmd/cli/state_apply.go#L159) 在 error_code 非 OK 时保留 phaseLogin + LastError | pass | `TestPlayerJourney_G11_NonOkLoginBlocks` |
| `[L1.3]` | MUST | LoginResp OK 后 user_id 必须写入 `RoomView.UserID` | [applyLogin](../../../cmd/cli/state_apply.go#L172) | pass | 既有 `TestApplyLogin*` |
| `[L2.1]` | MUST | 大厅主屏四张入口卡片 + ←→↑↓ + Enter；Esc 行为按 Esc 模型独立 feature 待设计 | [scene_lobby.go renderHome](../../../cmd/cli/scene_lobby.go) | pass | 既有 `TestSceneRouterRenderLobby` |
| `[L2.3]` | SHOULD | 大厅 UI 不出现协议 ID（rule_id / page_token / req_id） | `LobbyRoomMeta.RuleID` 仅作内部字段，渲染走 RuleMeta.DisplayName | pass | `TestPlayerJourney_L2_3_LobbyHasNoProtocolIDs` |
| `[L3.1]` | MUST | AutoMatch 不得把玩家匹入 `state ∈ {playing, settling, closed}` 的房间 | local 与 remote 路径均 ListRooms → SnapshotRoom 探活 → 跳过非 waiting/ready 房；详见 §2 | **修复就绪** | `TestPlayerJourney_L3_1_RoomAcceptsAutoMatchLocal`（本地 6 子用例）+ `TestRemoteRoomGatewayAutoMatchSkipsStartedRoom`（远端） |
| `[L3.2]` | MUST | AutoMatch 找不到可加入现房时必须 CreateRoom 并占座 0 | local fallback + remote fallback 路径已实现 | pass | `TestPlayerJourney_L3_2_AutoMatchFallsBackToCreateRoomLocal` |
| `[L3.3]` | MUST | AutoMatch 成功必须按事件序：JoinRoomResp/CreateRoomResp → 事件流订阅成功 → 下一帧 SceneRouter 切到 Prep/Table | `cmd/cli/scene.go::CurrentSceneID` 仅在 `Phase==phaseTable && RoomID!=""` 切换 | pass（与 `TestSceneRouterPlaysOneRoundWithBots` 一致） | 既有 `TestSceneRouter*` |
| `[L4.1]` | MUST | 公开房间列表来自 `ListRoomsResponse`；翻页仅显示 `< / >` 与页码 | [renderRooms](../../../cmd/cli/scene_lobby.go) 走 LobbyRoomList，没有 page_token 文案 | pass | 已纳入 L2.3 |
| `[L4.2]` | MUST | 列表每行显示房名、规则可读名、`n/4`、状态可读化 | LobbyRoomMeta + RuleMeta 投影；状态来自 RoomMeta.Stage | pass | `TestPlayerJourney_L2_3_LobbyHasNoProtocolIDs` 间接覆盖 |
| `[L5.2]` | MUST | 私密房创建后 `room_id` 在预备页顶栏持续显示直到 playing | [A3](../../spec/architecture-gaps.md#a3) 未修：`SceneRoomPrep` 顶栏未挂私密码 | **fail** | 留给 prep_segment（A3 已登记） |
| `[L6.1]` | MUST | 房间码加入是唯一保留文本输入的入口 | [renderJoinCode](../../../cmd/cli/scene_lobby.go) | pass | 既有 |
| `[L8.1]` | MUST | 改名走 `RenameRequest`，OK 后才更新本地 + SaveConfig | [scene_lobby.go::renderSettings](../../../cmd/cli/scene_lobby.go) | pass | 既有 |
| `[L9.1]` | DEFERRED | Esc 统一交互模型独立 feature 待设计 | n/a | n/a | 待 Esc 模型设计后补 |
| `[L10.1]` | MUST | 服务端版本不兼容（ROUTE_REDIRECT 等顶层错误）必须显式提示并阻断 | [applyLogin](../../../cmd/cli/state_apply.go#L160) 在 ROUTE_REDIRECT 翻起 Reconnecting + LastError | pass | `TestPlayerJourney_L10_1_RouteRedirectFlagsReconnecting` |

## 2. 关键发现

### 现象（修前）

- `internal/handler/local_gateway.go::AutoMatch` 旧版本直接调用 `lobby.AutoMatch`，根本不查 FSM 状态。又因为 `internal/service/lobby/lobby.go::roomMeta.stage()` 始终返回 `"waiting"`，`lobby.ListRooms` 不会过滤已开局/已结算/已关闭的房间，导致玩家可能被塞进 `state=playing/settling/closed` 的房。
- `internal/app/gate_remote.go::AutoMatch` 旧版本走 `clusterv1.LobbyServiceClient.AutoMatch`，由 lobby 单服决定，同样未透传 FSM 状态，回 cli 后看到的"已开局房间"。

### 根因

- 大厅 `roomMeta.stage()` 是占位实现（始终 `waiting`），没有任何上游推进它。短期内不准备做"lobby ↔ room FSM 状态同步"这种跨边界一致性改造（涉及 ADR-0014 决策 5 与 ADR-0044 决策 1，是结构性议题）。
- 因此 AutoMatch 必须在加入前自行向房服探活（`rooms.RoomSnapshot` 本地 / `clusterv1.RoomServiceClient.SnapshotRoom` 远端），把状态判定放在网关层，避免协议字段含义滑移。

### 修复（本轮）

- `internal/handler/local_gateway.go`：`AutoMatch` 改为 `ListRooms → roomAcceptsAutoMatchLocal (rooms.RoomSnapshot 探活) → lobby.JoinRoom → 再次探活 → rooms.Join → 返回`；任何一步不满足 → `LeaveRoom` + 继续；全部失败 → fallback 到 `CreateRoomWithMeta` + `rooms.Join`。
- `internal/app/gate_remote.go`（用户未提交 diff，本轮一并复核）：`AutoMatch` 改为 `ListRooms → joinLobbyRoom → roomAcceptsAutoMatch (SnapshotRoom 探活) → EnsureRoomEventSubscription → rememberRoomSeat → 返回`；探活失败 → `leaveLobbyRoom` + 继续；fallback 走 `CreateRoom`。
- 抽出 `roomStateProbe` 接口（[internal/handler/local_gateway.go::roomStateProbe](../../../internal/handler/local_gateway.go)），把 `roomAcceptsAutoMatchLocal` 单独可测；6 个 FSM 状态全部表驱动断言。

### 协议视角的边界

- L3.1 暴露的更大一层结构性议题：lobby 元数据中 stage 字段事实上没有可信来源。短期靠 AutoMatch 网关探活兜底；长期建议在 ADR-0044 决策 1 的"五类事实"里把 RoomLifecycle 的权威下沉到 room service，并要求 lobby ListRooms 通过 SnapshotRoom 派生 stage（或显式标 `stage=unknown` 强制下游探活）。此条作为新的 architecture-gap 候选，先记到 [A2](../../spec/architecture-gaps.md#a2) 的后续讨论，不在本轮纳入 spec MUST。

## 3. 修复跟踪

- [x] A2 `[L3.1]` / `[L3.2]`：local + remote 两条路径对齐，单元测试覆盖；详见上文。
- [x] A12 `[L2.3]`：lobby 渲染层 grep 测试 `TestPlayerJourney_L2_3_LobbyHasNoProtocolIDs` 锁定。
- [x] A13 `[G11]` / `[L10.1]`：applyLogin reducer 路径补 `TestPlayerJourney_G11_NonOkLoginBlocks` + `TestPlayerJourney_L10_1_RouteRedirectFlagsReconnecting`。
- [ ] A3 `[L5.2]` / `[P4.2]` 私密房房间码持续展示：留到 `prep_segment` 处理。
- [ ] 长期 architecture：lobby stage 权威来源下沉（候选议题，等用户决定是否进 ADR）。

## 4. 留底素材

- 演练驱动器：`LSP_JOURNEY_DRIVE=1 go test -run TestPlayerJourneyAgainstRealBackend ./cmd/cli/...`，每次跑会写 `frames.jsonl` 到 `t.TempDir()`；驱动器在测试结束时通过 `t.Logf` 打印帧 dump 路径。
- 锁定单测：`go test -run 'TestPlayerJourney_L|TestPlayerJourney_G11|TestPlayerJourney_L10|TestRemoteRoomGatewayAutoMatchSkipsStartedRoom' ./...`
