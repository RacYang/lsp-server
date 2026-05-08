# lsp-cli TUI 后端契约缺口清单

本文面向后端实现 agent。当前 `lsp-cli` 全屏 TUI 首版所需的阻塞契约已经补齐：规则列表、结构化最后动作、结构化副露、权威剩牌数、行动 deadline、座位在线/托管状态、局号与累计分都已具备前端消费入口。

## 1. 分类标准

- **A 类**:阻塞前端启动。没有这些契约，玩家关键路径不能按设计实现。
- **B 类**:增强体验。不阻塞首版 TUI，但会减少前端兜底逻辑。
- **C 类**:可延后。进入后续体验增强。

每一项都包含现状、玩家场景、建议契约、前端消费方式和临时降级。

## 2. A 类:阻塞项

当前没有 A 类阻塞项。

已完成的首版关键契约：

| 契约 | 当前状态 | 前端用途 |
| --- | --- | --- |
| 规则列表 | `ListRulesRequest` / `ListRulesResponse` 已加入 `client.v1` 与 `LobbyService` | 大厅创建房间向导 |
| 结构化最后动作 | `ActionDetail` / `LastActionInfo` / `SnapshotNotify.last_action` | 桌心最近一打、最近事件 |
| 结构化副露 | `MeldInfo` / `SeatMelds` / `SnapshotNotify.meld_infos_by_seat` | 四家副露轨道 |
| 权威剩牌数 | `wall_remaining` 已出现在开局、摸牌、动作、快照 | 桌心剩牌 |
| 行动 deadline | `deadline_unix_ms` 已出现在摸牌、动作、快照 | 倒计时与抢答进度 |
| 座位状态 | `SeatInfo.online` / `auto_play` / `disconnected_at_ms` / `status` | 座位在线、托管、断线 |
| 局号与累计分 | `round_index` / `hand_index` / `total_scores` | 顶栏、结算、玩家栏 |

## 3. B 类:增强体验

### B1. 再来一局语义

- **现状**:前端可沿用 `LeaveRoom + AutoMatch` 完成再来一局，但这会重新进入匹配池，不等价于同桌续局。
- **玩家场景**:结算后四名玩家希望留在同一桌继续下一局。
- **建议契约**:
  - 方案一：明确 `ReadyRequest` 在结算后代表同桌续局。
  - 方案二：新增 `RematchRequest` / `RematchResponse`，并在 `SettlementNotify` 后接受。
- **前端消费方式**:结算底栏 `r 再来一局` 调对应 gateway。
- **临时降级**:首版继续使用 `LeaveRoom + AutoMatch`，文案写成"再开一桌"而不是"同桌再来"。

### B2. `SeatInfo.status` 枚举固化

- **现状**:`SeatInfo.status` 是字符串，前端需要猜测合法值。
- **玩家场景**:座位状态需要稳定显示为空座、在线、离线、托管、投降、已胡。
- **建议契约**:
  - 新增 `SeatStatus` enum，或在协议文档中固化字符串集合。
  - 建议值：`empty`、`online`、`offline`、`auto_play`、`surrendered`、`hu`。
- **前端消费方式**:座位图和牌桌四角显示密度块与短标签。
- **临时降级**:优先使用 `online`、`auto_play`、`is_bot`、`surrendered` 布尔字段，`status` 只作日志辅助。

### B3. `RoomMeta.stage` 玩家可读化

- **现状**:`RoomMeta.stage` 是自由字符串。
- **玩家场景**:公开房列表需要显示"等待中"、"进行中"、"结算中"、"已关闭"。
- **建议契约**:
  - 固化 stage 字符串集合，或新增 enum。
  - 建议值：`waiting`、`playing`、`settling`、`closed`。
- **前端消费方式**:公开房列表的状态列。
- **临时降级**:未知值显示为"未知"，不展示协议原文。

### B4. 规则特性标签字典

- **现状**:`RuleMeta.enabled_features` 已存在，但 feature key 仍是机器字符串。
- **玩家场景**:规则卡片应显示"换三张"、"定缺"、"血战到底"等可读标签。
- **建议契约**:
  - 在协议文档中固化 feature key。
  - 或新增本地化标签映射。
- **前端消费方式**:大厅规则卡片、玩法帮助、牌桌右侧规则摘要。
- **临时降级**:前端内置当前两条川麻规则的标签映射。

## 4. C 类:可延后

### C1. 聊天频道

- **影响 UI**:桌外右侧栏或底部临时输入。
- **建议契约**:房间消息 notify 与发送请求。
- **临时降级**:首版不支持聊天。

### C2. 观战与回放

- **影响 UI**:非玩家视角、隐藏信息投影、事件游标。
- **建议契约**:观战 seat、可见信息等级、回放事件流。
- **临时降级**:首版不支持观战。

### C3. 断线动效细分

- **影响 UI**:网络浮层展示重连阶段。
- **建议契约**:连接阶段、重试次数、预计下次重试时间。
- **临时降级**:前端只显示在线、重连中、离线。

## 5. 后端 agent 建议顺序

1. 先确认再来一局语义，决定复用 `ReadyRequest` 还是新增 `RematchRequest`。
2. 固化 `SeatInfo.status` 与 `RoomMeta.stage` 的值域。
3. 固化 `enabled_features` 的 key 与玩家可读标签。
4. 再考虑聊天、观战和断线动效。

## 6. 前端临时降级规则

- 再来一局首版写作"再开一桌"，使用现有 `LeaveRoom + AutoMatch`。
- `status` 和 `stage` 未知时只显示通用状态，不泄露原始协议值。
- feature key 未知时隐藏该标签。
- 后端新增增强字段后，前端应优先使用后端事实，删除本地映射或估算。

## 7. 交接检查清单

后端完成任一增强项时，需要同步提供：

- proto 字段或消息名称。
- 字段含义和单位。
- 是否出现在 `SnapshotNotify` 中。
- 旧客户端兼容策略。
- 至少一个服务端测试或集成测试覆盖。
- 前端消费示例。
