# lsp-cli TUI 后端契约缺口清单

本文面向后端实现 agent。它只列出为了支撑新版 `lsp-cli` 俯视麻将桌 TUI 所需确认或补充的后端契约，不要求前端 agent 实现这些内容。

## 1. 分类标准

- **A 类**:阻塞真实牌桌。没有这些契约，前端只能猜，中心牌桌会出现不稳定或不真实的表现。
- **B 类**:增强体验。不阻塞中心牌桌首版，但会影响桌外辅助和玩家理解成本。
- **C 类**:可延后。不进入首轮 TUI。

每一项都包含：

- 缺口名称。
- 影响 UI 区域。
- 当前前端是否可降级。
- 需要新增或确认的 proto 字段 / notify。
- 后端完成后的前端消费方式。

## 2. A 类:阻塞真实牌桌

### A1. 结构化副露

- **影响 UI 区域**:中心牌桌四家副露轨道。
- **当前前端是否可降级**:可弱降级。当前 `RoomView.Players[].Melds` 是 `[]string`，例如 `pong:p5` / `gang:m1`，只能画简化副露，不能可靠表示来源、明暗、杠类型和多张牌列表。
- **需要契约**:
  - 新增结构化 `MeldInfo`，包含 `seat_index`、`kind`、`tiles`、`claimed_from_seat`、`concealed`、`step`。
  - `SnapshotNotify.melds_by_seat` 当前是 `SeatTiles` 文本编码，建议新增 `repeated SeatMelds meld_infos_by_seat`，保留旧字段兼容。
  - `ActionNotify` 的 `pong` / `gang` 动作建议携带结构化副露，或新增 `MeldNotify`。
- **前端消费方式**:
  - `TableScene.Seats[].Melds` 直接使用结构化牌组。
  - 暗杠/明杠/补杠可用不同黑白排列表达。
  - 来源座位可决定副露牌横向/侧向摆放。

### A2. 结构化最后动作

- **影响 UI 区域**:桌心最近一打、右侧最近事件、抢答浮层上下文。
- **当前前端是否可降级**:弱降级。当前前端只能从 `ActionNotify` 和最后一张弃牌推断，不足以恢复重连后的最近动作，也无法区分吃碰杠胡、摸牌、抢答窗口来源。
- **需要契约**:
  - 新增 `LastActionInfo`，包含 `step`、`actor_seat`、`action`、`tile`、`target_seat`、`source_seat`、`created_at_ms`。
  - `SnapshotNotify` 增加 `last_action`。
  - `ActionNotify` 可逐步结构化，或新增并行字段 `ActionDetail detail`。
- **前端消费方式**:
  - 桌心优先展示 `last_action.tile`。
  - 右侧栏显示 `actor + action + tile`。
  - 抢答窗口标题使用 `source_seat` 和 `tile`，不从日志猜。

### A3. 权威剩牌数

- **影响 UI 区域**:桌心剩牌数、海底提示。
- **当前前端是否可降级**:可估算但不应放入桌内当权威事实。现有 `remainingTilesEstimate` 用手牌、弃牌、副露估算，重连、暗杠、补牌、血战多人胡牌后可能不准。
- **需要契约**:
  - `StartGameNotify`、`DrawTileNotify`、`ActionNotify`、`SnapshotNotify` 增加 `wall_remaining` 或 `remaining_tiles`。
  - 若规则存在补杠、杠后补牌，也应由后端统一维护剩牌。
- **前端消费方式**:
  - 桌心显示权威 `remaining_tiles`。
  - `<= 8` 时启用海底提示。
  - 字段缺失时隐藏桌心剩牌或标为估算并退到桌外。

### A4. 服务端行动 deadline

- **影响 UI 区域**:桌心计时、抢答倒计时、底栏超时提示。
- **当前前端是否可降级**:可弱降级。当前前端用 `ActionStartedAt + 固定时长`，本地时钟、网络延迟和服务端托管推进会导致倒计时不准。
- **需要契约**:
  - 等待动作相关 notify 增加 `deadline_unix_ms` 或 `remaining_ms`。
  - `SnapshotNotify` 增加当前等待动作的 deadline。
  - claim 窗口需要每个候选自己的 deadline，避免接力抢答时显示错。
- **前端消费方式**:
  - `InteractionModel.Timer` 使用服务端 deadline。
  - 本地仅做展示倒计时，不决定权威超时。
  - 重连后从 `SnapshotNotify` 恢复倒计时。

### A5. 玩家连接/托管状态

- **影响 UI 区域**:桌内座位状态、左侧玩家列表、断线重连提示。
- **当前前端是否可降级**:部分可用。`SeatInfo.surrendered` 被用作托管/投降类状态，但缺少 online、disconnected、auto_play 等明确状态。
- **需要契约**:
  - 扩展 `SeatInfo`，新增 `online`、`auto_play`、`disconnected_at_ms`、`status`。
  - 或新增 `SeatStatusNotify`，广播座位状态变化。
- **前端消费方式**:
  - 左侧栏显示 `● 在线`、`○ 离线`、`⏸ 托管`。
  - 桌内只做最小图标提示，不写长句。
  - 自己断线恢复仍由本地连接状态和快照驱动。

## 3. B 类:增强体验

### B1. 权威局号/场次

- **影响 UI 区域**:顶部 modeline、结算摘要。
- **当前前端是否可降级**:可降级为本地会话计数，显示为 `本次会话第 N 局`。
- **需要契约**:
  - `StartGameNotify` / `SnapshotNotify` 增加 `round_index`、`hand_index` 或规则内局号。
- **前端消费方式**:
  - 顶部显示 `第 N 局`。
  - 重连后保持一致。

### B2. 实时积分

- **影响 UI 区域**:左侧玩家状态、右侧战绩趋势、结算前局势判断。
- **当前前端是否可降级**:可不显示。结算后可本地累计，但不权威。
- **需要契约**:
  - `SeatInfo` 或 `SnapshotNotify` 增加 `score` / `total_score`。
  - `SettlementNotify` 后端继续下发每家得失。
- **前端消费方式**:
  - 左侧栏显示当前积分。
  - 右侧栏显示近局趋势。

### B3. 权威听牌/可胡提示

- **影响 UI 区域**:右侧听牌建议。
- **当前前端是否可降级**:可客户端估算，且只能作为辅助。
- **需要契约**:
  - 若要权威提示，新增 `HintNotify` 或在 `SnapshotNotify` 中增加 `available_waits`、`discard_to_waits`。
  - 需明确是否只给自己。
- **前端消费方式**:
  - 右侧栏显示 `听: X Y` 或 `打 X -> 听 Y`。
  - 标注为规则提示或服务端提示。

### B4. 规则可读元数据

- **影响 UI 区域**:大厅房间卡片、右侧规则摘要、帮助浮层。
- **当前前端是否可降级**:可用本地 `ruleDisplayNames` 映射，但不够完整。
- **需要契约**:
  - 房间/规则元数据增加 `display_name`、`short_desc`、`enabled_features`。
  - 至少覆盖换三张、定缺、血战到底、番种限制。
- **前端消费方式**:
  - 大厅和右侧栏展示可读规则。
  - 避免客户端硬编码过多规则文案。

### B5. 牌墙/剩余牌池摘要

- **影响 UI 区域**:右侧危险提示、海底提示。
- **当前前端是否可降级**:可启发式估算。
- **需要契约**:
  - 如需权威危险/剩张，后端需按规则决定是否可泄露信息。
  - 可只下发公开可推导的 `visible_remaining_by_tile`。
- **前端消费方式**:
  - 右侧栏显示剩张，不进入桌内。

## 4. C 类:可延后

### C1. 头像/称号/段位

- **影响 UI 区域**:左侧玩家状态。
- **当前前端是否可降级**:显示昵称即可。
- **需要契约**:用户资料扩展。
- **前端消费方式**:左侧栏增强显示。

### C2. 观战/回放

- **影响 UI 区域**:非首轮目标。
- **当前前端是否可降级**:不支持。
- **需要契约**:观战座位、隐藏信息投影、事件回放游标。
- **前端消费方式**:未来单独设计。

### C3. 多局房间总积分榜

- **影响 UI 区域**:右侧战绩、结算页。
- **当前前端是否可降级**:本地累计。
- **需要契约**:房间级累计分。
- **前端消费方式**:结算页与右侧栏展示。

### C4. 动画事件细分

- **影响 UI 区域**:出牌、胡牌、杠牌动画。
- **当前前端是否可降级**:TUI 首轮只做状态稳定。
- **需要契约**:更细的事件流。
- **前端消费方式**:未来增强。

## 5. 建议给后端 agent 的处理顺序

1. 确认是否扩展现有 `client.v1`，以及如何遵守 proto baseline。
2. 先补 A2 结构化最后动作和 A4 行动 deadline。
3. 再补 A1 结构化副露。
4. 再补 A3 权威剩牌数。
5. 最后补 A5 玩家状态。
6. B 类按产品优先级补，C 类暂不处理。

## 5.1 当前落地状态

- A1 结构化副露：已追加 `MeldInfo` / `SeatMelds` 与 `SnapshotNotify.meld_infos_by_seat`，旧 `melds_by_seat` 保留兼容。
- A2 结构化最后动作：已追加 `ActionDetail` 与 `SnapshotNotify.last_action`，room engine 在出牌、碰、杠、胡时写入权威动作。
- A3 权威剩牌数：已追加 `wall_remaining` 并在摸牌、动作与快照投影中下发。
- A4 服务端行动 deadline：已追加 `deadline_unix_ms`，快照从 room scheduler 的权威 deadline 投影；实时 notify 字段已预留并随动作链路透传。
- A5 玩家连接/托管状态：`SeatInfo` 已追加 `online`、`auto_play`、`disconnected_at_ms`、`status`。
- B1/B2/B4：已追加 `round_index`、`hand_index`、`total_scores` 与 `RuleMeta`；B3 听牌提示保留为后续 `HintPolicy` 能力。

## 6. 前端临时降级规则

在后端完成前，前端实现应遵守：

- 没有结构化副露时，只画 `pong:p5` / `gang:m1` 的简化牌组。
- 没有结构化最后动作时，桌心最近一打只在实时 `ActionNotify(discard)` 后展示，重连后可为空。
- 没有权威剩牌数时，桌心不显示估算值，或把估算放右侧栏。
- 没有 deadline 时，倒计时只作为本地辅助，不能作为权威超时。
- 没有玩家在线状态时，只显示已知 `is_bot` / `surrendered`。

## 7. 交接检查清单

后端 agent 完成任一缺口时，需要同步提供：

- proto 字段或 notify 名称。
- 字段含义和单位。
- 是否出现在 `SnapshotNotify` 中。
- 旧客户端兼容策略。
- 至少一个服务端测试或集成测试覆盖。
- 前端消费示例。
