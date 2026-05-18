# 玩家旅程标准（player-journey spec）

本文件是 `lsp-server` 真人对局玩家旅程的明文标准。任何"对/标/优秀/玩家期望"的争议都以本文件条款编号（例如 `[L3.1]`、`[D4.2]`）为准。drills 切片、回归测试名、`docs/spec/architecture-gaps.md` 中的架构缺陷条目都必须引用这里的编号。

## 修订记录

- v0.5 · 2026-05-12：Esc 统一交互模型作为独立 feature 暂不推进。v0.3 引入的"Esc 分层"被撤销；与 Esc 直接相关的 4 条（`[G2]` Esc 部分、`[L2.1]` 退出键说明、`[L9.1]`、`[P3.3]`）改为 DEFERRED 占位。cli 退出统一兜底为 `Ctrl-C` / 关终端，不再绑定任何应用层快捷键。
- v0.4 · 2026-05-12：托管作为独立 feature 暂不推进；本 spec 直接以"托管不存在"为硬契约：出牌超时不再有"服务端兜底代打"，统一按 surrender 处理；任何残留的"服务端代玩家做动作"路径都作为 architecture-gap 登记并要求关闭。
- v0.3 · 2026-05-12：按用户裁决：退出键迁至 Esc 分层、彻底去掉"托管"语义、听牌升 MUST、零和升严、私密房码改"创建后必须在预备页醒目持续展示"。
- v0.2 · 2026-05-12：根据 self-audit 订正硬编码时延、私密房码事实、补 `PassRequest` / `RenameRequest` / 杠类型 / 隐私边界 / 流局亮牌 / 中途离桌 / 终端尺寸 / 主题降级 / 编号约定与已知反例附录。
- v0.1 · 2026-05-12：初稿。

## 范围声明：托管 (auto-play) 暂不实现

托管是一项完整功能（玩家短暂离开期间由服务端按策略代打、玩家回归时取消托管、托管下的副露/胡牌策略、托管玩家的结算责任等）。本 spec 不规定它的行为；本版本以"**托管不存在**"为契约：

- 客户端不渲染托管态（`[G12]`）。
- 服务端不得在出牌 / 抢答 / 自摸 / 暗杠等任何节点替玩家做出有意义的局内决策；超时一律 surrender 该席或显式 pass（`[D2.4]`、`[C2.2]`、`[C3.3]`、`[T1.2]`）。
- 任何当前实现里仍然存在的"服务端代打"路径，登记到 `docs/spec/architecture-gaps.md` 的 "AUTO_PLAY-OFF" 区块，要求显式关闭（feature flag / 配置降级），不在本轮 P0 实现完整托管功能。
- 托管未来作为独立 feature 重新设计时，新增对应 spec 章节（编号另起，例如 `[A1..An]`），并把这一段范围声明缩窄。

## 范围声明：Esc 统一交互模型暂不实现

Esc 一旦绑定了"退出 cli / 返回 / 打开局内菜单"等多重职责，就是一套完整交互模型（菜单结构、与抢答冲突的优先级、弃局警告浮层、跨场景一致性）。v0.3 引入的"Esc 分层"经 v0.5 评估属于设计深度不足而引入的不一致风险，已撤销；该统一模型作为独立 feature 延后整体设计。本版本以下列最小契约为准：

- cli 不在应用层绑定任何"退出"快捷键。退出 cli 走操作系统级路径：`Ctrl-C` 或关闭终端窗口。退出仍受 `[G10]` 约束（若进程退出时仍在房间内，必须尽力 `LeaveRoom` 落地，落地失败 stderr 告警）。
- Esc 在子页 / 浮窗中的"返回上一级 / 关闭浮窗"沿用当前 cli 行为（创建向导、加入码、公开房列表等子页 Esc 返回主页）。本 spec 不再对"Esc 在每个场景做什么"作 MUST 规定；保留各 Scene 现有 `HandleKey` 中的 Esc 处理。
- `q` 不绑定任何全局动作（避免与玩家输入字符冲突）；后续 Esc 模型设计完成前不引入新的全局退出/菜单键。
- 与 Esc 直接相关的 4 条条款（`[G2]` Esc 部分、`[L2.1]` 退出键说明、`[L9.1]`、`[P3.3]`）在 v0.5 改为 DEFERRED 占位；待 Esc 模型独立 feature 出炉后以新 `[E1..En]` 章节合并。
- 任何当前实现里"按 Esc 在某场景做了破坏性动作"的现象（如直接退出 / 直接 LeaveRoom 而无确认），均登记到 architecture-gaps 的 "ESC-MODEL-DEFERRED" 区块。

## 0. 用法

- **对 (MUST)**：违反即视为缺陷，必须根因修复。绑定到 ADR-0044 五类事实之一，或绑定到 ADR-0039 的局内权威字段。
- **标 (SHOULD)**：违反不阻塞功能但偏离玩家在常见川麻客户端形成的直觉；缺一条就在 drills 留痕，不一定立刻修。
- **优秀 (MAY)**：达到才算"好玩"。失败不在本轮 P0 范围。
- **玩家期望**：玩家在该节点用自然语言会怎么描述自己的诉求；用于反推 UXTransient 文案。

每条 MUST 必须可由"事实字段断言 + 帧文本断言 + 事件先后断言"中的至少一项验证。spec 未覆盖的现象一律先回补 spec，再决定要不要改实现。

## 0.1 标准依据

- 玩法（外部权威）：
  - [维基百科 · 四川麻將](https://zh.wikipedia.org/wiki/%E5%9B%9B%E5%B7%9D%E9%BA%BB%E5%B0%87)
  - [四川麻将游戏规则详解（血战到底/血流成河）](https://cns-scmajianggame.com/rules)
  - 茶苑《血战麻将换三张玩法技巧口诀》
  - 同城游《血战到底规则·换三张麻将规则》
- 内部契约：
  - [ADR-0039 四川血战权威局内契约](../adr/0039-sichuan-xuezhandaodi-authoritative-round-contract.md)
  - [ADR-0040 组合式麻将规则能力](../adr/0040-composable-mahjong-rule-capabilities.md)
  - [ADR-0044 房间状态与前后端交互契约](../adr/0044-room-state-and-client-contract.md)
  - [ROOM-FSM](../ROOM-FSM.md)、[RULE-ENGINE](../RULE-ENGINE.md)、[FRONTEND-DESIGN](../FRONTEND-DESIGN.md)、[cli-tui-backend-gaps](../cli-tui-backend-gaps.md)
- runtime 参数（dev.yaml 当前值）：`runtime.room.surrender_action_timeout=1s`、`runtime.room.surrender_after_offline=30s`、`runtime.room.allow_leave_during_play=true`、`runtime.lobby.bot_supervisor_enabled=true`、`runtime.lobby.max_bots_per_room=3`。spec 中提到"长离线阈值"等参数均以 runtime 配置为权威，不在 spec 中重写数值。

## 0.2 全局不变量

- `[G1] (MUST)` TUI 所有显示项必须可溯源到 ADR-0044 的五类事实之一；禁止从协议日志文本或 `LastDiscard` 反推。
- `[G2] (MUST)` 输入许可分两层。**对局动作级键**（出牌 Enter、抢答 h/g/p/n、换三张 Space、定缺 m/p/s 等）仅当 `view.SeatIndex ∈ RoundProgress.acting_seats && available_actions 含目标动作` 时生效；`RoundPhase` 只描述阶段，不参与输入许可。**场景级键**（`Tab`、`?`、主题切换、子页 `Esc` 返回等）由当前 `SceneID` 决定，沿用当前 cli 实现；统一交互模型作为独立 feature 待设计，v0.5 不在 spec 中规定 Esc 的具体行为。
- `[G3] (MUST)` 同一份状态事实，本地模式（`cmd/all` 内 LocalGateway）与集群模式（`cmd/gate` + remoteGateway）必须等价。任一侧字段缺失视为协议层缺陷。
- `[G4] (MUST)` cli 不得改写服务端 `SeatInfo`、`RoundProgress`、`RoundFacts`、`SettlementNotify` 中的任何字段，亦不得"本地预投影"未确认动作（pending 灰显属于 UXTransient，不写回事实）。
- `[G5] (SHOULD)` 任意按键到 TUI 视觉反馈必须在一帧内（下一个 `SceneRouter.Render` 即可见 pending/光标变化）；服务端确认到帧更新必须在收到对应响应/通知后的下一帧内。本条以"帧序事件断言"判定，不以墙钟毫秒判定。
- `[G6] (MAY)` 关键阶段切换（开局、定缺完成、抢答弹层、结算）可以加视觉强调（高光/边框/反白），强调不得阻塞下一帧输入。
- `[G7] (MUST)` 牌桌渲染必须满足 `MinTableWidth × MinTableHeight`（`cmd/cli/table_layout.go`）；不满足时进入牌桌前以居中文案拒绝并提示具体最小尺寸，且玩家放大窗口后无须重新登录即可继续。
- `[G8] (MUST)` 牌面主题必须支持降级链 Unicode → CJK → ASCII；降级仅影响 `tile_art` 模块，不得影响输入许可或事实投影。
- `[G9] (MUST)` 隐私投影必须 per-seat：本家摸牌明牌仅对本家可见；他家暗杠在第三方视角只显示"暗杠"动作，不暴露具体牌；抢杠/流局亮牌按 ADR-0039 决策 4 通过权威投影下发，cli 不得本地推断。
- `[G10] (MUST)` 玩家显式退出（`Ctrl-C` 或关闭终端窗口；应用层退出键由 Esc 模型独立 feature 决定）必须先尝试完成 `LeaveRoom`（若在房间内）；放弃落地仅在网络真的不可达时允许，且必须在下一帧 stderr 输出明确告警。
- `[G11] (MUST)` 任何 ErrorCode 非 OK 的 LoginResp 必须阻断后续大厅/牌桌操作并在登录页显示可读错误；不得静默继续。
- `[G12] (MUST)` **托管功能在本版本不存在**。客户端不得渲染 `SeatInfo.auto_play` 字段为玩家可见的"托管"图标；座位状态值域仅限：`● 在线 / ○ 离线 / ▲ 弃局 / ✓ 已胡 / ▣ 机器人 / □ 空座`。服务端不得在任何节点替玩家做有意义的局内决策；超时一律 surrender 或显式 pass（见 `[D2.4]` / `[C2.2]` / `[C3.3]` / `[T1.2]`）。
- `[G13] (MUST)` 长离线达 `runtime.room.surrender_after_offline` 必须直接判 `surrendered`（弃局），不进入任何"托管"中间态；该席从摸打轮转中移除，桌继续运转。
- `[G14] (MUST)` 结算零和：`SettlementNotify.seat_scores` 全部 `total_fan` 与 `penalties` 全部 `delta` 的算术和必须为 0；不为零视为服务端结算缺陷，回归测试必须直接 fail 并 dump 详情（不再"以权威值为准"模糊处理）。

---

## 1. 大厅 L

玩家期望："输个名字就能上桌，不用看 'rule_id' 这种东西。"

- `[L1.1] (MUST)` 启动后若 `~/.lsp/config.toml` 存在 `SessionToken`，必须以静默续登路径进入大厅；玩家最多看到一行"连接中..."。
- `[L1.2] (MUST)` 续登失败（LoginResp 含 `error_code` 非 OK、token 过期、网络不可达、服务端版本不兼容）必须落回登录页要求重输昵称；不得以 "终端玩家" 兜底继续。
- `[L1.3] (MUST)` 服务端返回 `client.v1.LoginResp` 后必须把 `user_id` 写入 `RoomView.UserID`；客户端对后续所有协议字段（含 `winner_user_ids`）以此为锚。
- `[L2.1] (MUST)` 大厅主屏显示四张入口卡片（与现 cli 一致）：快速开始 / 创建房间 / 加入房间码 / 公开房间。键位 `←→/↑↓` 选、`Enter` 确认；`Esc` 在子页"返回上一级"沿用 cli 现状（创建向导 / 加入码 / 公开房列表 / 设置 / 帮助），主页 `Esc` 行为由 Esc 模型独立 feature 决定，本 spec 不规定。退出 cli 通过 `Ctrl-C` 或关闭终端窗口（见 `[G10]`）。
- `[L2.2] (MUST)` 顶栏必须显示：昵称、服务器、连接状态指示与 RTT。
- `[L2.3] (SHOULD)` 大厅 UI 不得出现任何协议 ID（`rule_id`、`page_token`、`req_id` 等）。
- `[L3.1] (MUST)` `AutoMatch` 不得把玩家匹入 `RoomLifecycle ∈ {playing, settling, closed}` 的房间。事实来源：`cluster.v1 SnapshotRoom.state` 或 `client.v1 RoomMeta.stage`。
- `[L3.2] (MUST)` `AutoMatch` 找不到可加入现房时必须新建公开房并占座 0；玩家无需额外确认。
- `[L3.3] (MUST)` `AutoMatch` 成功必须满足三件事按事件序发生：①收到 `JoinRoomResp` 或 `CreateRoomResp`、②订阅该房间事件流成功、③下一帧 `SceneRouter.CurrentSceneID()` 切到 `SceneRoomPrep`/`SceneTable`。任一缺失视为缺陷。
- `[L3.4] (SHOULD)` 订阅事件流失败必须在底栏显示并自动重试，不得静默吞错。
- `[L4.1] (MUST)` 公开房间列表来自 `ListRoomsResponse`；翻页仅显示 `< / >` 与页码，禁止显示 `page_token`。
- `[L4.2] (MUST)` 列表每行显示房名（`RoomMeta.display_name`）、规则可读名（`RuleMeta.display_name`）、人数 `n/4`、状态可读化（等待中 / 进行中 / 结算中 / 已关闭，映射自 `RoomMeta.stage`）。
- `[L4.3] (SHOULD)` 已开局/已关闭房不可点击进入；尝试加入必须给出可读拒绝原因并保留列表选区。
- `[L5.1] (MUST)` 创建房间是三步向导：规则 → 公开/私密 → 房名。规则卡片只显示 `RuleMeta.display_name / short_desc / enabled_features / max_hands`。
- `[L5.2] (MUST)` 私密房（`CreateRoomRequest.private=true`）创建成功后必须把 `CreateRoomResponse.room_id` 作为"房间码"在 `SceneRoomPrep` 顶栏醒目持续显示，直到房间进入 `playing`；长度由服务端决定，cli 不得截断或本地伪造格式。本条与 `[P4.2]` 中的"私密房房间码"部分对齐升为 MUST。
- `[L5.3] (SHOULD)` 公开房默认房名为 `规则名 · 昵称`；玩家按 Enter 可直接接受。
- `[L6.1] (MUST)` 房间码加入是唯一保留文本输入的入口；输入校验非空与长度，错误只显示在底栏。
- `[L6.2] (MUST)` 加入失败原因（不存在 / 已开局 / 已满 / 私密房不可见）必须落到 UXTransient 文案，不得直接抛协议原文。
- `[L7.1] (MUST)` 服务端踢出 / 路由重定向 / 限流降级必须在大厅顶栏或浮窗即时显示，且不刷掉玩家当前选区。
- `[L8.1] (MUST)` 改名通过 `RenameRequest`；本地 `Config.Nickname` 仅在收到 `RenameResponse{error_code=OK}` 后才更新，并 `SaveConfig`。
- `[L8.2] (SHOULD)` 改名失败必须以可读原因显示在大厅底栏，原昵称保留。
- `[L9.1] (DEFERRED)` 原"退出键位统一为 Esc 分层"方案撤销。退出 cli 在 v0.5 仅依赖操作系统级 `Ctrl-C` / 关终端，并受 `[G10]` 约束；应用层退出键 / 局内菜单 / 跨场景统一交互模型作为独立 feature 待设计，定稿后以新 `[E*]` 章节合并并取代本占位条款。本占位期间，各场景对 Esc 的处理沿用 cli 现有 `HandleKey` 行为，spec 不作 MUST 规定。
- `[L10.1] (MUST)` 服务端版本不兼容（由 `LoginResp.error_code` / 顶层错误标志携带）必须显式提示并阻断后续操作，提示包含玩家可执行的下一步（升级 cli / 切换服务器）。

## 2. 房间预备 P

玩家期望："我已经进房了，看看谁来了；不齐就等机器人补上，齐了就开局。"

- `[P1.1] (MUST)` 进房后必须立即可见四个座位（含空座），自家固定在底部；座位状态完全由 `SeatInfo` 投影到 `[G12]` 定义的值域：`● 在线 / ○ 离线 / ▲ 弃局 / ✓ 已胡 / ▣ 机器人 / □ 空座`。不得渲染"托管"状态。
- `[P1.2] (MUST)` cli 在 ADR-0044 决策 7 约束下不得本地写 ready、不得本地标 bot；新增机器人必须等服务端 `SeatInfo` 更新。
- `[P2.1] (MUST)` `runtime.lobby.bot_supervisor_enabled=true` 时，房间停留 `waiting` 超过 supervisor 内置阈值后必须由 lobby supervisor 自动 `AddBot` 直至凑齐 4 人（上限受 `runtime.lobby.max_bots_per_room` 约束）。
- `[P2.2] (SHOULD)` 玩家按 `b` 显式 `AddBot` 时，必须立刻反馈"请求已发送"，并在 `SeatInfo` 实际更新后才标该座位为 `▣`。
- `[P3.1] (MUST)` 玩家进房后客户端必须立刻发 `ReadyRequest`，玩家无需手按；服务端拒绝 ready（例如非可 ready 阶段）必须在底栏可读提示。
- `[P3.2] (MUST)` 四人齐 + 全 ready 后必须在 ≤ 一帧内观察到 `RoomLifecycle: waiting → ready → playing`；超过 1 个事件循环仍未推进必须在底栏提示原因。
- `[P3.3] (SHOULD)` 玩家在 `playing` 之前可主动离桌（走 `LeaveRoom` 真请求，不仅本地切场景）；具体触发键位待 Esc 模型独立 feature 定稿后引用 `[E*]`，v0.5 不在此处规定。
- `[P4.1] (MUST)` 底栏键位随 `RoomLifecycle` 切换：`waiting` 显示离桌/补位，`ready` 显示"等待开局"；任何牌桌阶段键位不得在预备页显示。
- `[P4.2] (MUST/SHOULD)` 顶栏必须显示：私密房房间码（**MUST**，对应 `[L5.2]`）、规则可读名（SHOULD）、`round_index / max_hands`（SHOULD）、累计积分（SHOULD）。私密房房间码不出现视为缺陷。
- `[P5.1] (MUST)` 玩家在预备页可改名（`RenameRequest`）；改名成功后服务端必须重发该桌 `SeatInfo` 更新让全桌可见。

## 3. 换三张 E

玩家期望："开局先换三张，我要看着自己的 13 张牌挑 3 张同花色出去。"

- `[E1.1] (MUST)` `RoundProgress.waiting_action=exchange_three` 时本家必须看到完整 13 张权威手牌；任何错位视为缺陷。
- `[E1.2] (MUST)` 选 3 张必须强制同一花色（万/筒/条）；UI 在选第二张异花色时即拒绝标记，不依赖服务端兜底。
- `[E2.1] (MUST)` 键位：`←/→` 移光标、`Space` 标记 / 取消、`Enter` 在 `已选 3/3` 时提交。底栏必须实时显示 `已选 N/3`。
- `[E2.2] (MUST)` 服务端拒绝（`ExchangeThreeResponse.error_code` 非 OK）必须把原因落到 UXTransient notice，并保留 marked 状态让玩家改选。
- `[E3.1] (MUST)` 服务端按 `RoundState.exchangeDirection`（ADR-0039 决策 2）执行交换；客户端不得自行猜方向或本地复刻交换结果。
- `[E3.2] (MUST)` `ExchangeThreeDoneNotify` 到达后，本家手牌必须由服务端权威投影更新（旧 3 张消失 + 新 3 张出现），cli 不得生成过渡帧。
- `[E4.1] (SHOULD)` 三家完成后桌心或顶栏短暂提示"换三张完成"，提示不得阻塞下一帧输入。
- `[E4.2] (MAY)` 换牌瞬间允许新牌入手动画 / 高光。

## 4. 定缺 Q

玩家期望："换完牌该选我打哪一门；选完不可改，所以提示要清楚。"

- `[Q1.1] (MUST)` `waiting_action=que_men` 时按 `m / p / s` 提交缺万 / 缺筒 / 缺条；其它键忽略且无副作用。
- `[Q1.2] (MUST)` 提交后 SeatRoster 必须出现本家定缺标记（服务端权威字段），全桌可见。
- `[Q1.3] (MUST)` 全四家提交才进入摸打循环；任何一家未提交时本家不得开始摸牌。
- `[Q2.1] (SHOULD)` 提示文案必须明确告诉玩家"选定后不可更改"。
- `[Q2.2] (SHOULD)` 定缺完成后 TUI 必须在自家手牌区视觉上区分缺门花色（例如灰显），帮助玩家执行"优先打缺门"。

## 5. 摸打循环 D

玩家期望："轮到我摸牌时画面立刻告诉我摸到啥；轮到我打牌时按方向键选、按 Enter 出。"

- `[D1.1] (MUST)` `RoundPhase=PHASE_DRAW` 仅作为 UXTransient 派生信息使用（如"摸牌中"占位），**不得**写入 `RoomView.WaitingAction`（ADR-0044 决策 2/3）。
- `[D1.2] (MUST)` 本家收到 `DrawTileNotify`（包含明牌）后，下一帧自家手牌必须可见新牌，光标默认停在新摸牌位置（除非已存在 pending 出牌）。
- `[D1.3] (MUST)` 他家摸牌广播只携带"动作"，不得包含明牌（per-seat privacy，`[G9]`、ADR-0039 决策 4）。
- `[D2.1] (MUST)` 本家可出牌当且仅当 `waiting_action=discard && view.SeatIndex ∈ acting_seats && available_actions 含 discard`；不满足时 Enter 无效且不弹错误。
- `[D2.2] (MUST)` 按 Enter 出牌请求发出后，下一帧该牌必须在自家手牌变灰（pending），且重复按 Enter 不重复发送（防抖）。
- `[D2.3] (MUST)` 收到对应 `DiscardResponse{error_code=OK}` / `ActionNotify(self=discard)` 后的下一帧，自家手牌总数必须 -1 且 pending 清除；收到非 OK 必须恢复手牌并显示可读拒绝原因。
- `[D2.4] (MUST)` 出牌超时按 surrender 处理：达 `runtime.room.surrender_action_timeout` 后服务端必须直接判该席 `surrendered`（弃局），不得替玩家选任何具体牌；该席退出本局摸打轮转，桌内剩余玩家按 ROOM-FSM 继续推进。客户端按 `[G13]` 渲染该席为 `▲ 弃局`。
- `[D3.1] (SHOULD)` 桌心必须显示最近一打（`SnapshotNotify.last_action` / `ActionNotify.detail`），含座位方位与玩家昵称。
- `[D3.2] (SHOULD)` 自家牌河、他家牌河必须按 `RoundFacts.discards` 投影；不得用按键回放估算。
- `[D4.1] (MUST)` 已胡座位（`hued_seats` 含）不参与摸打轮转；TUI 在该座位显示"已胡"明确状态。
- `[D4.2] (MUST)` 牌墙剩牌数（`wall_remaining`）必须实时显示且与权威字段相等；零和耗尽时进入流局路径而非继续摸。
- `[D5.1] (MUST)` 杠的四种形态（直杠/明杠/暗杠/补杠）必须由服务端 `MeldInfo` 区分；TUI 副露轨道按形态展示，不混淆。
- `[D5.2] (MUST)` 暗杠在他家视角只显示"暗杠"占位，不暴露具体牌（与 `[G9]` 联动）。
- `[D6.1] (SHOULD)` 海底牌（`wall_remaining=0` 摸牌）、杠上花、杠上炮、抢杠胡等上下文必须由服务端 `ScoringContext` 标记，TUI 在结算与最近动作中显式标注，不靠 cli 推断。
- `[D7.1] (MUST)` 听牌（"叫"）状态必须由服务端权威下发（来源：`internal/mahjong/hu/ting.go` 已具备听牌算法），并在本家进入叫状态时由 TUI 在桌心或自家手牌区给出显式提示，且在结算前持续可见。当前 `client.v1` 协议**没有**承载该事实的字段，登记为 architecture-gap "协议待补 Tenpai/Listening 投影"，未补齐前回归测试至少断言：服务端日志中"听牌就绪"事件存在且对应玩家可在 TUI 看到对应文案。

## 6. 抢答（碰/杠/胡/过）C

玩家期望："别人打的牌我能碰/杠/胡就立刻看到浮窗，倒计时清楚；选一个或过掉。"

- `[C1.1] (MUST)` 抢答候选完全由 `RoundProgress.claim_candidates / available_actions` 驱动；cli 不得用 `LastDiscard` 反推弹窗对象。
- `[C1.2] (MUST)` 弹窗仅对 `claim_candidates` 列出的座位呈现；其它座位不得显示弹窗。
- `[C2.1] (MUST)` 弹窗倒计时来自 `deadline_unix_ms`；进度条 / 数字必须严格用它换算。
- `[C2.2] (MUST)` 抢答窗口超时时由客户端在 `deadline_unix_ms` 前显式发 `PassRequest`（"过"），服务端不得代替玩家选"碰/杠/胡"。客户端若已离线导致 `PassRequest` 不能发出，服务端直接判该席 `surrendered` 并跳过该次抢答（与 `[D2.4]` 同语义）。
- `[C3.1] (MUST)` 键位：`←→` 切按钮、`Enter` 确认；快捷键 `h / g / p / n` 直选胡 / 杠 / 碰 / 过。
- `[C3.2] (MUST)` 过期窗口（`claim_window_open=false`）不得在客户端重新打开（ROOM-FSM "过牌后" 不变量）。
- `[C3.3] (MUST)` 显式"过"必须发 `PassRequest`；自动兜底超时也必须发 `PassRequest`（不依赖服务端默认）。
- `[C4.1] (MUST)` 接力抢答按 ROOM-FSM "胡 > 杠 > 碰" 优先级显示候选与裁决；cli 不得本地裁决。
- `[C4.2] (SHOULD)` 一炮多响时弹窗必须对每个可胡者独立出现；任一家"过"不影响其他家。
- `[C5.1] (MUST)` 抢答成功（碰/杠）后必须再等本家显式 `DiscardRequest`（ADR-0039 决策 1）；客户端不得伪造下一动作。
- `[C5.2] (MUST)` 抢杠胡按 ADR-0039 +1 番并使被抢杠的明杠无效；cli 只读结算，不参与计算。
- `[C5.3] (MUST)` 抢杠胡的被抢杠者必须在其视角看到"杠被抢"明确状态（`ActionNotify.detail` 或专用字段），不得只显示"杠失败"。

## 7. 自摸窗口 T

玩家期望："我摸到一张能胡的牌，先问我胡不胡；不胡就让我接着选打哪张。"

- `[T1.1] (MUST)` 摸牌后立即可胡时必须先进入 `waiting_action=tsumo_window` 弹窗，按钮为 `胡 / 不胡`。
- `[T1.2] (MUST)` 主动胡走结算路径；主动不胡必须由客户端显式发 `PassRequest`，服务端不得代选"胡/不胡"；选"不胡"后摸到的牌入手，本家继续 `waiting_action=discard`。客户端离线导致 `PassRequest` 不能发出时，服务端直接判 `surrendered`。
- `[T2.1] (MUST)` 自摸窗口倒计时与 `[C2.1]` 同源，使用 `deadline_unix_ms`。
- `[T2.2] (SHOULD)` 默认建议高亮"胡"按钮。
- `[T3.1] (MUST)` 海底/杠上/杠上炮等上下文由服务端 `ScoringContext` 标记；cli 仅读取 `SettlementNotify` 字段展示。
- `[T4.1] (MUST)` 暗杠后摸牌（杠上摸花）仍走 `tsumo_window` 同源逻辑；他家视角不见明牌（与 `[G9]`、`[D5.2]` 联动）。

## 8. 结算 S

玩家期望："这局打完了，我想知道谁赢、我赢多少、为什么。"

- `[S1.1] (MUST)` `SettlementNotify` 到达必须立即弹结算浮窗；显示赢家、本家得失、累计积分、番种列表。
- `[S1.2] (MUST)` 命令行同时输出文本摘要（沿用 `snapshotSettlementSummary`），便于复盘。
- `[S2.1] (MUST)` 累计积分必须取 `SettlementNotify.total_scores` / `SeatInfo.total_score`，不得本地累加。
- `[S2.2] (MUST)` 番种文案来自 `per_winner_breakdown.fan_names`（必要时叠加 `RuleMeta.feature_labels`）；不得 cli 硬编码番种名。
- `[S2.3] (MUST)` "你赢了 / 你输了 / 平" 判定基于"本家 user_id 是否在 `winner_user_ids` 列表"。
- `[S3.1] (MUST)` 多家胡（一炮多响、血战连续胡）必须把每家拆分清楚显示，而不是只显示"赢家=第一个胡者"。
- `[S4.1] (MUST)` 流局必须显式说明"流局"，并按 `SettlementNotify.penalties` 显示查叫 / 花猪赔付 / 退税。
- `[S5.1] (MUST)` 底栏键位固定为：`r 再开一桌 / l 离桌 / Enter 停留`。
- `[S6.1] (SHOULD)` 流局时所有未胡座位的最终手牌按服务端权威投影亮出，便于查叫复盘；亮牌前他家手牌仍按 `[G9]` 隐藏。
- `[S7.1] (MUST)` 严格零和（与 `[G14]` 同源）：`SettlementNotify.seat_scores` 全部 `total_fan` 与 `penalties` 全部 `delta` 的算术和必须为 0；不为零视为服务端结算缺陷，回归测试直接 fail 并 dump `seat_scores`、`penalties`、`per_winner_breakdown` 详情；cli 显示与权威值若有差异，先在 drills 留痕再定位 reducer / 投影根因。

## 9. 再开一桌 R

玩家期望："这桌打完想再来一局；不在乎是不是同桌，但别让我又匹回这桌或者卡在原房。"

- `[R1.1] (MUST)` 按 `r` 必须先 `LeaveRoom` 真请求成功，再 `AutoMatch`；不得仅本地切场景。
- `[R1.2] (MUST)` `AutoMatch` 必须遵守 `[L3.1]`：不匹回原房（原房此时 `state=settling/closed`）。
- `[R1.3] (MUST)` 多局房间 `max_hands > 1` 时由服务端按 ROOM-FSM `playing → settling → waiting` 推进；cli 在收到下一局 `playing` 信号前不得发起再开一桌。
- `[R1.4] (MUST)` 多局之间累计积分（`total_score`）必须保留；本局 ready / surrendered 状态被清除时 cli 必须在顶栏短暂提示"准备开始下一局"，让玩家不会误以为分丢了。
- `[R2.1] (SHOULD)` 按 `l` 必须回到大厅并保留昵称与最近房间码（私密时）。
- `[R2.2] (SHOULD)` 按 `Enter` 必须停留在结算页，玩家可慢慢看番种。
- `[R3.1] (MUST)` `runtime.room.allow_leave_during_play=true` 时，玩家在 playing 中按"返回大厅"必须以"投降/弃局"语义走 `LeaveRoom`，服务端将其座位置为 `surrendered`；cli 必须显式提示玩家"离桌将判为弃局"以确认操作。
- `[R3.2] (MUST)` `runtime.room.allow_leave_during_play=false` 时，playing 中按"返回大厅"必须被拒绝并给出可读原因；不在本地静默切回大厅。

## 10. 断线与重连 N

玩家期望："不想因为 5 秒断网丢整局；真断了，给我看清楚状态。"

- `[N1.1] (MUST)` 短暂断线（≤ heartbeat 周期 × 3）：顶栏出现 `○ 重连中`，不清空当前场景。
- `[N1.2] (MUST)` 重连成功必须以 `SnapshotRoom` / `SnapshotNotify` 为权威恢复座位、用户、阶段；任何字段不得"漂移"（user_id 错位、seat_index 移位等）。
- `[N1.3] (MUST)` 重连后必须按 `SnapshotNotify.last_step` 切点丢弃陈旧增量（ADR-0039 决策 3）。
- `[N2.1] (MUST)` 长离线（达到 `runtime.room.surrender_after_offline`）必须在浮窗增加"返回大厅"按钮且可用；按下走 `LeaveRoom` + 回大厅，并按 `[R3.1]` 提示弃局影响。
- `[N2.2] (SHOULD)` `RouteRedirectNotify` 必须在顶栏显著提示"服务端切换网关"，并自动重连到新地址。
- `[N3.1] (DELETED)` 原"托管恢复"条款已删除。按 `[G12]` / `[G13]`：cli 不渲染托管态；长离线达 `surrender_after_offline` 直接判弃局，重连后看到的是 `▲ 弃局` 而非"托管"。
- `[N4.1] (MUST)` 服务端版本不兼容（重连返回的 LoginResp 不再可用）必须按 `[L10.1]` 走阻断路径，而非反复重连。

---

## 11. spec 与 ADR-0044 五类事实的映射

| 事实类 | 对应条款（节选） |
| --- | --- |
| RoomLifecycle | `[L3.1] [L4.2] [P3.2] [P4.1] [R1.3] [R3.1] [G13]` |
| RoundProgress | `[D1.1] [D2.1] [D2.4] [C1.1] [C3.2] [T1.1] [D7.1]` |
| SeatRoster | `[P1.1] [P1.2] [P2.1] [P5.1] [Q1.2] [D4.1] [G12] [G13]` |
| RoundFacts | `[E1.1] [E3.2] [D1.2] [D3.2] [D4.2] [D5.1] [D5.2] [D6.1] [S1.1] [S2.1] [S3.1] [S4.1] [S6.1] [S7.1] [G14]` |
| UXTransient | `[L6.2] [E2.2] [Q2.1] [Q2.2] [E4.1] [T2.2] [R1.4] [G6]`（`[L9.1]` 已 DEFERRED） |

任一条款若无法映射到上表中的某一类事实，需要回 spec 评审并明确"是新增协议事实，还是仅 UXTransient 文案"。

## 12. 验收断言来源约定

- **事实字段断言**：直接读 `RoomView` 或 `SnapshotNotify` 的对应字段。
- **帧文本断言**：用 `simulationScreenText` 抓帧，对关键提示词做 substring 断言。
- **事件序断言**：用 `clock.Clock` 注入的固定时钟，断言"事件 A 发生后下一帧 B 必须成立"，不用墙钟毫秒断言。

回归测试名格式：`TestPlayerJourney_<条款>_<描述>`，例如 `TestPlayerJourney_L3_1_AutoMatchSkipsPlayingRoom`、`TestPlayerJourney_D2_3_DiscardAckReducesHand`。

## 13. 边界声明

- 本 spec 仅覆盖 P0 玩家旅程；观战 / 回放 / 聊天 / 多区域路由等延后项不写入 MUST。
- 本 spec 不替代 ADR；若与现有 ADR 冲突，以 ADR 为准并在本文件留痕，再决定是否提案 ADR 演进。
- 本 spec 由用户审稿确认后才作为后续 drills 与回归测试的硬标准。

---

## 附录 A：已知反例（用于回归测试设计）

下列现象在近期 commit 修过又复发，每个反例都绑定一条 spec 条款；驱动器测试名应直接以这些条款编号命名，避免"无锚点"修复。

- `9736178 fix(room): 冻结摸牌通知投影状态` → 反 `[D1.1]` / `[D1.2]`：摸牌通知一度污染 `WaitingAction` 且导致手牌错位。回归断言：`view.WaitingAction != "draw"` 且新摸牌出现在自家手牌。
- `636fbaa fix(cli): 防止座位手牌错位` → 反 `[E1.1]` / `[D1.2]`：换三张/摸打过程中自家手牌错位。回归断言：自家 seat_index 与 user_id 在整局不漂移，手牌索引稳定。
- `936e82e fix(cli): 修复牌桌交互与机器人同步` → 反 `[P1.1]` / `[P1.2]`：机器人补位与本地座位状态投影不一致。回归断言：所有 SeatInfo 字段不被 cli 本地写回。
- `4a6231c fix(cli): 修复牌桌状态同步与机器人回填` → 反 `[G3]` / `[G4]`：本地/集群事件信息量不等价。回归断言：local 与 cluster 模式下同 trace_id 事件序列字段集合相等。
- `f6e765a fix: 修复换三张手牌同步与交互提示` → 反 `[E3.2]` / `[E2.2]`：换三张完成后客户端手牌未按权威投影刷新或拒绝原因被吞。回归断言：`ExchangeThreeDoneNotify` 后下一帧手牌按权威投影替换。
- `a86afc9 fix(cli): 消除出牌失败提示测试竞态` → 反 `[D2.3]`：出牌失败时手牌恢复与提示链有竞态。回归断言：在所有 `DiscardResponse.error_code != OK` 用例下，下一帧手牌恢复且 notice 非空。
- 当前未提交 diff（`internal/app/gate_remote.go` + `internal/handler/local_gateway.go` 的 AutoMatch 探活） → 反 `[L3.1]`：AutoMatch 把玩家塞进 playing 房。回归断言：local 与 remote 两条 gateway 路径均通过 `SnapshotRoom`/`stage` 跳过 playing/settling 房，并能在没有空房时回落到 CreateRoom。
- 现状：`cmd/cli/scene_lobby.go` 私密房创建后直接进入预备页，无显式房间码展示步骤 → 反 `[L5.2]` / `[P4.2]` 的 MUST 部分。回归断言：私密房创建后下一帧在 `SceneRoomPrep` 顶栏可见 `room_id` 子串，且持续可见直到 `RoomLifecycle=playing`。
- 现状：`client.v1` 没有 tenpai/listening 字段 → 反 `[D7.1]`。回归断言：服务端 round projector 已能识别听牌（`internal/mahjong/hu/ting.go`），但 TUI 帧文本无法读到对应提示。架构 gap：协议待补 `tenpai_by_seat`。
- 现状：服务端 `SeatInfo.auto_play` 字段仍存在 → 反 `[G12]`。回归断言：cli 帧文本不得出现"托管"字样，且 `SeatRoster` 投影函数对 `auto_play=true` 必须降级到"在线/离线"。架构 gap：协议长期演进决定是否删字段。
- 现状：服务端长离线策略与 cli 展示边界不一致（前者可能仍走"托管"，后者按 `[G12]` 只显示弃局）→ 反 `[G13]`。回归断言：达到 `surrender_after_offline` 时 `SeatInfo.status` 必须含 `surrendered`，cli 渲染 `▲ 弃局`。
- AUTO_PLAY-OFF：服务端在出牌超时 / 抢答超时 / 自摸窗口超时等节点替玩家做"具体动作"的兜底路径必须被关闭或显式降级到 `surrender / pass`，不在本轮 P0 实现完整托管功能。涉及代码：`internal/service/room` 的 surrender/timeout 推进入口、`runtime.room.surrender_action_timeout` 的语义、bot supervisor 的"占座代打"边界（这是机器人，非托管，要确保两套不被串）。回归断言：在玩家未发出任何动作的连续 N 个超时窗口内，没有任何 `ActionNotify(source=player, kind=discard)` 来自该 user_id，且 `SeatInfo.status` 在第一次超时即变为 `surrendered`。
- ESC-MODEL-DEFERRED：Esc 统一交互模型未实现期间，任何场景下"按 Esc 即触发破坏性动作（直接退出 cli、直接 LeaveRoom 而无确认、直接切场景丢失状态）"的现象都登记在此。回归断言：本占位期间各 Scene 的 `HandleKey` 中 Esc 路径不得引入新的"退出 cli / LeaveRoom / 跨场景跳转"逻辑；仅允许保留当前行为（子页返回 / 关闭浮窗）。Esc 模型独立 feature 定稿后本区块整体撤销，并替换为新 `[E*]` 章节对应的回归。

附录 A 不是穷举，发现新的回归条目时直接在此追加，并补对应 spec 条款编号。

## 附录 B：编号约定

- 条款编号一旦发布即视为契约，**只增不复用**：删除条款时保留编号并加 `[DEPRECATED]` 标记，避免历史 drills/测试名失锚。
- 新增条款按段内最大编号 +1，跨段编号互不影响。
- 条款合并必须保留旧编号为"别名"指向新编号一个版本周期。
