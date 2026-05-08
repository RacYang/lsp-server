# lsp-cli TUI 前端设计包

本文定义新版 `lsp-cli` 的玩家端终端界面。范围覆盖大厅、房间预备、牌桌、结算、设置与网络恢复；目标是让玩家按游戏语义行动，而不是按协议字段操作。

## 1. 玩家旅程

```text
启动客户端
  -> 静默续登或登录
  -> 大厅
  -> 快速开始 / 创建房间 / 加入房间码 / 查看公开房
  -> 房间预备
  -> 换三张
  -> 定缺
  -> 摸打循环
  -> 碰 / 杠 / 胡 / 过
  -> 结算
  -> 再来一局 / 返回大厅 / 离桌
```

| 阶段 | 主焦点 | 辅助信息 | 底栏动作 |
| --- | --- | --- | --- |
| 大厅 | 三张入口卡片 | 昵称、服务器、RTT、最近房间 | `←→` 选项、`Enter` 确认、`?` 帮助 |
| 创建房间 | 规则卡片向导 | 规则简介、公开/私密说明 | `←→` 切换、`Enter` 下一步、`Esc` 返回 |
| 房间预备 | 四座座位图 | 房间码、规则、机器人状态 | `Enter` 准备、`b` 补机器人、`q` 返回 |
| 牌桌 | 居中俯视牌桌 | 玩家详情、最近事件、听牌辅助 | 当前阶段动作键 |
| 结算 | 结算清单 | 总分、番种、房间摘要 | `r` 再来一局、`l` 离桌、`Enter` 停留 |
| 断线 | 原场景 + 网络浮层 | 重连状态、最近错误 | `Esc` 菜单，长时离线可返回大厅 |

## 2. 信息边界

大厅和牌桌都必须使用玩家语言。`rule_id`、`page_token`、`req_id` 等协议字段不得出现在 UI 文案中。

### 大厅允许

- 快速开始、创建房间、加入房间码、公开房间。
- 规则可读名、规则简介、人数、房间状态。
- 昵称、服务器、RTT、当前主题。

### 牌桌允许

- 四家手牌或牌背。
- 四家牌河。
- 四家副露。
- 最近一打。
- 剩牌、倒计时、庄位、局号。
- 方位、短昵称、在线/托管/机器人状态。

### 结算允许

- 胜负、番数、番种、罚分。
- 每家本局得失与累计分。
- 再来一局、返回大厅、离桌。

### 禁止项

- 创建房间时要求玩家手输规则。
- 用协议 ID 当菜单项。
- 桌内出现长句教学文案或大段历史日志。
- 用侧栏挤偏中心牌桌。
- 用 Unicode 牌图标的显示宽度承担布局。

## 3. 全局视觉系统

1. **黑白可读**：核心语义不依赖颜色，使用反白、粗细、边框和密度块。
2. **真实俯视**：自己永远在南；下家在左；对家在北；上家在右。
3. **选择优先**：可枚举选项都用方向键选择，只有昵称和房间码保留文本输入。
4. **阶段化底栏**：底部只展示当前阶段最相关的 3 到 5 个键。
5. **图标可降级**：牌面优先 Unicode Mahjong Tiles，其次 CJK，最后 ASCII。

终端 cell 视觉近似为 `1:2`，所以视觉正方形牌桌在实现上满足 `TableFrame.Width == TableFrame.Height * 2`。

| 档位 | 终端 | 中心牌桌 | 桌外信息 |
| --- | ---: | ---: | --- |
| Standard | `100x30` | `52x26` | 隐藏侧栏，只保留顶栏和底栏 |
| Wide | `120x36` | `64x32` | 右侧显示关键辅助 |
| Full | `140x40` | `72x36` | 左右侧栏都显示 |

## 4. 大厅设计

大厅必须是全屏 TUI，而不是 `stdin/stdout` 问答。

### 主屏

```text
┌ lsp · 大厅 · racoo · ● 23ms ───────────────────────────────────────┐
│                                                                    │
│        ┌ 快速开始 ┐     ┌ 创建房间 ┐     ┌ 加入房间码 ┐             │
│        │ 自动补齐 │     │ 选规则开局│     │ 好友邀请   │             │
│        │ Enter    │     │ Enter     │     │ Enter      │             │
│                                                                    │
│ 公开房间                                                            │
│  > Alice 的局        四川血战到底（换三张）    2/4    等待中        │
│    Bob 的局          四川血战到底（标准）      1/4    等待中        │
│                                                                    │
│ ←→/↑↓ 选择    Enter 确认    n 改名    s 设置    ? 帮助    q 退出     │
└────────────────────────────────────────────────────────────────────┘
```

### 创建房间向导

创建房间是三步向导：

1. 选择规则：调用 `ListRules` 获取 `RuleMeta`，展示 `display_name`、`short_desc`、`enabled_features`、`max_hands`。玩家看不到 `rule_id`。
2. 选择公开性：`公开房间` / `私密房间` 左右切换。私密房间展示"创建后复制房间码给朋友"。
3. 房间名：默认值为 `规则名 · 昵称`，玩家按 `Enter` 接受；只有想改名时才输入文本。

当前后端已提供两个规则：

| 玩家可读名 | 协议 ID | 展示方式 |
| --- | --- | --- |
| 四川血战到底（换三张） | `sichuan_xuezhandaodi_huansanzhang` | 首选卡片 |
| 四川血战到底（标准） | `sichuan_xuezhandaodi_biaozhun` | 次选卡片 |

### 公开房间

公开房间列表展示房间名、规则名、人数、状态。分页只用 `<` / `>` 和页码表达，绝不展示 `page_token`。

### 加入房间码

房间码是唯一保留文本输入的路径。输入前提示格式，输入后校验非空和长度；错误只在底栏显示。

### 设置与帮助

设置用键盘菜单，不使用 `y/N` 问答。帮助用 `?` 打开覆盖层，内容按当前场景动态裁剪。

## 5. 房间预备

房间预备是"已进房但未开局"的独立场景。中心区域显示四座座位图，右侧显示房间码与规则摘要。

| 状态 | 视觉 |
| --- | --- |
| 真人在线 | `● 昵称` |
| 离线 | `○ 昵称` |
| 托管 | `◐ 昵称` |
| 机器人 | `▣ 机器人` |
| 空位 | `□ 空座` |

字段来源为 `SeatInfo.is_bot`、`SeatInfo.online`、`SeatInfo.auto_play`、`SeatInfo.disconnected_at_ms`、`SeatInfo.status`。

## 6. 牌桌设计

牌桌仍以居中正方形为第一优先级。桌外信息不足时隐藏侧栏，而不是压缩牌桌。

桌内对象：

- 南：自己的手牌、牌河、副露。
- 西、北、东：他家牌背、牌河、副露。
- 桌心：最近一打、剩牌、倒计时、庄位。
- 四角或短边：短昵称、在线/托管/机器人状态。

协议字段使用：

| UI | 字段 |
| --- | --- |
| 最近一打 | `SnapshotNotify.last_action` / `ActionNotify.detail` |
| 剩牌 | `StartGameNotify.wall_remaining` / `DrawTileNotify.wall_remaining` / `ActionNotify.wall_remaining` / `SnapshotNotify.wall_remaining` |
| 倒计时 | `DrawTileNotify.deadline_unix_ms` / `ActionNotify.deadline_unix_ms` / `SnapshotNotify.deadline_unix_ms` |
| 局号 | `round_index` / `hand_index` |
| 积分 | `SeatInfo.total_score` / `SnapshotNotify.total_scores` |
| 副露 | `SnapshotNotify.meld_infos_by_seat`，旧字段只作兼容 |

## 7. 抢答、换三张与定缺

抢答弹层由 `claim_candidates` 和 `available_actions` 驱动；倒计时来自 `deadline_unix_ms`。超时只做展示，权威动作由服务端推进。

换三张固定为三张选择向导：

- `←→` 移动。
- `Space` 标记。
- `Enter` 提交。
- 底栏显示 `已选 N/3`。

定缺固定为三门选择：

- `m` 缺万。
- `p` 缺筒。
- `s` 缺条。

## 8. 结算与再来一局

结算场景显示：

- 总番：`SettlementNotify.total_fan`。
- 赢家拆分：`per_winner_breakdown`。
- 罚分：`penalties`。
- 每家本局得失：`seat_scores`。
- 累计分：`total_scores`。

底栏固定为：

```text
r 再开一桌    l 离桌    Enter 停留
```

首版再来一局沿用现有 `LeaveRoom + AutoMatch` 回路；如后端后续新增 `RematchRequest`，前端只替换 gateway 动作。

## 9. 网络与断线

网络状态是叠加层，不属于任何单一场景。

- 短暂断线：顶栏显示 `○ 重连中 Ns`。
- 长时断线：中央小浮层，不清空原场景。
- `RouteRedirectNotify`：顶栏显示"服务端切换网关"。
- 续登成功：根据快照 `phase` 自动回到大厅、预备、牌桌或结算，不强制回大厅。

## 10. 前端技术架构

```text
WebSocket Envelope
  -> Gateway
  -> AppState
  -> InteractionModel
  -> SceneRouter
  -> Scene
  -> tcell.Screen
```

`SceneRouter` 管理以下场景：

- `LobbyScene`
- `RoomPrepScene`
- `TableScene`
- `SettleScene`
- `SettingsScene`
- `ErrorScene`
- `NetOverlay`

场景接口只处理渲染、键盘事件和周期 tick。网络请求全部走 gateway，状态事实全部来自 `AppState`。

## 11. 数据契约盘点

| 数据 | 来源 | 渲染位置 |
| --- | --- | --- |
| 规则列表 | `ListRulesResponse.rules` | 大厅创建房间向导 |
| 房间列表 | `ListRoomsResponse.rooms` | 大厅公开房表格 |
| 座位 | `JoinRoomResponse.seats` / `SnapshotNotify.seats` | 房间预备与牌桌四家 |
| 自己手牌 | `InitialDealNotify` / `DrawTileNotify` / `SnapshotNotify.your_hand_tiles` | 南家手牌 |
| 牌河 | `ActionNotify` / `SnapshotNotify.discards_by_seat` | 四家牌河 |
| 副露 | `SnapshotNotify.meld_infos_by_seat` | 四家副露 |
| 当前动作 | `phase` / `acting_seats` / `waiting_action` | 焦点与底栏 |
| 抢答 | `claim_candidates` / `available_actions` | 抢答弹层 |
| 结算 | `SettlementNotify` | 结算场景 |
| 网络 | 连接状态 / `RouteRedirectNotify` | 顶栏与网络浮层 |

## 12. 降级策略

### 尺寸

- `140x40`: 两侧信息栏完整显示。
- `120x36`: 右侧关键辅助显示。
- `100x30`: 只显示中心牌桌、顶栏、底栏。
- 低于 `100x30`: 提示放大终端，不进入牌桌。

### 牌面

降级顺序为 Unicode Mahjong Tiles、CJK 中文牌、ASCII 牌。降级只改变牌面，不改变布局坐标。

### 协议字段

字段缺失时隐藏对应信息。客户端估算只能放桌外，并标注为辅助。

## 13. Fixture 与 Golden

fixture 必须固定 `RoomView`、场景、终端尺寸、牌面主题、当前时间和光标状态。

golden 分组：

- `lobby_*`: 大厅主屏、规则向导、公开房、设置。
- `room_prep_*`: 空座、三人、满员、机器人补位。
- `table_*`: 等待、换三张、定缺、摸打、抢答、断线。
- `settle_*`: 自摸、点炮、多赢家、荒局。
- `error_*`: 服务器不可用、规则列表不可用、房间码无效。

## 14. 验收清单

- 创建房间全程不出现 `rule_id`。
- 大厅、预备、牌桌、结算都在同一个 tcell 全屏内完成。
- 中心牌桌不被侧栏挤偏。
- 底栏只显示当前阶段动作。
- `last_action`、`wall_remaining`、`deadline_unix_ms`、`total_scores` 都有明确渲染位置。
- 断线恢复后根据快照回到正确场景。
- `make verify` 与 golden 测试通过。

## 15. 实现交接

实现顺序：

1. `SceneRouter` 与 `Gateway.ListRules`。
2. `LobbyScene` 与创建房间向导。
3. `RoomPrepScene`。
4. `TableScene` 接入新协议字段。
5. `SettleScene` 与再来一局。
6. `NetOverlay`。
7. 删除旧 `stdin/stdout` lobby。
