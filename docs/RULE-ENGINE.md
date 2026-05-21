# 规则引擎

## 目标

麻将变体规则必须可替换，而无需重写传输、存储或房间编排。

## 设计

规则接口分为注册元数据与能力集合。`rules.Rule` 只提供规则 ID 与展示名称；真实房间运行路径必须使用 `RuleCapabilitiesProvider.Capabilities()` 暴露的组合策略。房间层只保存通用局面事实、通用流水和 opaque 规则运行态：

- `TileSetPolicy` / `WallPolicy`：规则牌组、牌墙、补花、王牌、岭上牌等摸牌能力。
- `OpeningPolicy`：换三张、定缺、选庄等开局子流程及其协议投影。
- `ClaimPolicy` / `SelfActionPolicy`：吃、碰、明杠、暗杠、补杠、抢杠胡、胡等动作颗粒的可用性、互斥与优先级。
- `WinPolicy` / `TurnFlow`：胡牌合法性、摸牌、自摸窗口、杠后补牌、海底处理与已胡座位轮转。
- `ScoringPolicy` / `SettlementPolicy`：番种上下文、流水折叠、罚分、退税与累计积分。
- `TerminationPolicy`：三家胡、牌墙耗尽、血流等终局条件。
- `RoundProjection`：TUI/重连所需的结构化副露、最后动作、deadline、剩牌数与规则元数据。

组合边界见 [ADR-0040](adr/0040-composable-mahjong-rule-capabilities.md)。ECS/组合模式只作为拆分职责的参考；仓库不引入通用游戏框架。

## 注册表

变体按名称注册并通过配置选择。房间通过**接口**请求规则行为，而非按规则名分支。

`RoundState.caps` 是 room 内唯一的运行能力来源。房间在新建或恢复局面时装配一次 `CapabilitySet`；后续摸打、抢答、胡牌、计分、终局和投影都读取该能力快照，不在动作路径中反复从 `rules.Rule` 查询策略。

`SelfActionPolicy` 与 `ClaimPolicy`、`WinPolicy`、`ScoringPolicy` 一样是装配期必填能力。通用暗杠/补杠策略可以作为规则包显式选择的原子，但 room 不在动作路径里为缺失的 `SelfActions` 补默认实现。

`State`、`StateView`、`Turn`、`Settlement` 也是装配期必填能力。room 主流程不得用空规则状态、空投影、标准胡后退出或通用结算结果补救规则包错误；缺能力必须在 `CapabilitiesOf` 或生产规则能力测试中失败。

开局等待态在 room 内只保存为通用 opening wait，`PhaseUpdate.reason` 与 `phase` 也统一为 opening。`exchange_three`、`que_men` 或后续新增选庄/买马等动作名只来自 `OpeningPolicy.CurrentStep` 与 `rule_state` 投影，不能新增 room 专属等待字段、专属 waiting reason 或专属 phase。客户端、gate、handler 与 room actor 的提交入口统一为 `opening_action`，完成投影统一为 `opening_done`；room 只转发动作名、通用参数和规则包生成的 key/value 投影，具体含义由规则包 `OpeningPolicy.Apply` 与 `OpeningDoneProjection` 解释。

CLI、bot 和 supervisor 也遵守同一边界：它们可以在 adapter/策略层识别四川 action 并显示对应文案或选择牌张，但本地阶段模型和调度判断不得重新分裂出 exchange/que 专属 phase 或按这两个 action 写通用控制流。

结算统一经 `CapabilitySet.Settlement.BuildSettlement` 生成 `rules.SettlementResult`，再由 room service 投影为 `SettlementNotify`。胡后是否继续参与轮转统一经 `CapabilitySet.Turn.HuedSeatContinues` 决定。`RoundState` 使用 `rules.ScoreEvent` 记录通用计分流水；四川血战的查花猪、查大叫、退税等仍由四川规则包折叠，room 不再直接依赖四川结算结构。

旧持久化主字段已废弃，包括旧缺门、已胡座位、赢家列表、旧计分流水、累计番数和换三张提交态字段。`RestoreRoundFromPersistJSON` 只接受当前 `schema_version`，低版本、高版本或缺失版本都不可恢复，由上层降级重新准备。新 `round_json` 写出路径不得把这些字段作为主模型；缺门、换三张提交态等规则私有事实必须进入 `rule_state`，并由规则包解释。`RoundView.QueBySeat` 等客户端投影字段可以继续存在，但只能由权威投影派生，不能反向成为 room 运行状态。

`rule_state` 对 room 是 opaque payload：room 只保存 `schema_version` / `data` 并转交给策略。规则包通过 `RuleStateCodec` 初始化、规范化当前状态，通过 `RuleStateProjector` 输出通用 `SeatInts` 与 `OpeningSubmittedByAction` 投影；四川缺门只作为 `SeatInts["que_suit"]` 进入协议/视图 adapter。room 非测试代码不得读取 `MissingSuitBySeat`、`OpeningSelections` 或 `OpeningDirection` 这类规则私有字段。

## 规则 ID 命名

规则 ID 是配置、房间快照、日志、客户端规则摘要和测试夹具共同使用的稳定契约，命名见 [ADR-0041](adr/0041-mahjong-rule-id-naming.md)。

- 格式固定为 `<region>_<variant>_<option>`。
- 三段均使用小写全拼音，不使用首字母缩写，不混入英文译名。
- `option` 表示玩法选项；只有单一形态的规则也必须显式使用 `biaozhun`。
- Go 包路径是实现细节，不能替代 `rule_id`。

## 当前变体

当前注册三套规则：

- `guobiao_jingji_biaozhun`：国标竞技标准，完整 144 张牌组，支持字牌、花牌补花、吃碰杠、首胡即终局。
- `sichuan_xuezhandaodi_huansanzhang`：带换三张，支持换三张与定缺开局。
- `sichuan_xuezhandaodi_biaozhun`：不带换三张，直接进入定缺开局。

两套规则共享和牌、番种、结算与血战续行逻辑，差异由 `OpeningPolicy` 和规则元数据声明。当前四川血战实现支持：

- 定缺
- 无吃牌
- 和牌后血战续行
- 自摸、点炮胡与抢杠胡上下文
- 基础番种、海底/杠上/根与结构化结算

国标通过同一套房间流程运行，声明 `full_tiles`、`honors`、`flowers`、`mcr_81_fans`、`win_ends_round` 等能力。番种计算由国标规则包内的 MCR scorer 负责，覆盖 81 番 matcher、基础排除关系、8 分起胡、花牌计分和自摸/点炮结算；room 与 CLI 不推导国标计分。`mcr_81_fans` 的暴露由 registry 81 项、`primary/rawWant/scoredWant` fixture、代表性精确分值、排除关系和全仓回归共同守住。

## 规则来源与能力开关

- 国标 MCR：以 [EMA/WMO Green Book](https://mahjong-europe.org/portal/index.php?Itemid=167&id=31&option=com_content&view=article) 体系为实现依据；`mcr_81_fans` 表示当前规则包启用 81 番 registry、matcher 与排除图，且每个 fan 都有主目标 fixture。
- 日麻 Riichi：接口设计参考 [WRC Rules](https://www.worldriichi.org/wrc-rules)；本阶段只保留可接入边界，不实现立直、振听、宝牌、符/役和连庄。
- 港麻 HKOS/HKMA：接口按 136/144、花牌可选、起胡番数可配置、一炮多响/截胡可配置的形态预留；本阶段不实现完整港麻番表。
- `xuezhan_continue` 只表示胡后退出但牌局续行；`xueliu_multi_hu` 只允许在血流规则包和对应 conformance 测试完成后暴露。

## 新玩法接入清单

新增玩法必须只新增规则包、规则测试和必要协议投影 adapter：

1. 确定稳定 `rule_id`，并在规则包注册。
2. 显式实现 `RuleCapabilitiesProvider` 并提供 `TileSetPolicy`、`ClaimPolicy`、`SelfActionPolicy`、`WinPolicy`、`RuleStateCodec`、`RuleStateProjector`、`TurnFlow`、`ScoringPolicy`、`SettlementPolicy`、`TerminationPolicy`、`RoundProjection`；`CapabilitiesOf` 不补任何运行策略，缺能力即失败。
3. 用 conformance fake rule 或规则自身测试证明首胡终局、续行、多响、开局流程等差异不需要改 room 主流程。
4. 更新 `ListRules` 元数据和文档，feature flag 只能声明已通过夹具的能力。

房间编排负责 actor 串行化、等待态、托管超时与持久化恢复；规则包不得依赖传输层、存储层或 room service。

本轮多玩法重构的审查清单见 [多玩法麻将重构审查清单](multi-rule-refactor-review.md)。

## 局内契约

局内权威事实先进入 `RoundState` 或规则运行态，再投影到 client/cluster proto。TUI A/B 类字段包括：

- `last_action` / `ActionDetail`
- `deadline_unix_ms`
- `meld_infos_by_seat`
- `wall_remaining`
- `SeatInfo.status`、`online`、`auto_play`
- `round_index`、`hand_index`、`total_scores`
- `RuleMeta.enabled_features`

缺口状态与优先级见 [lsp-cli TUI 后端契约缺口清单](cli-tui-backend-gaps.md)。房间生命周期见 [ROOM-FSM](ROOM-FSM.md)。
