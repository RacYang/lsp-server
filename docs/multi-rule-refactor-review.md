# 多玩法麻将重构审查清单

## 审查目标

本轮改动把原先偏四川血战的房间流程收口为通用麻将局流程内核，并把玩法差异迁入规则策略包。审查时优先确认两件事：

- `internal/service/room` 只编排通用 opening、通用动作、事件、快照和投影，不重新引入具体玩法判断。
- 国标与川麻都通过同一套 room 主流程运行，差异只来自 `rule_id` 对应的策略能力。

## 建议提交边界

如果需要拆提交，按以下顺序拆分；每一段都应能独立说明行为变化和测试证据：

1. **牌模型与基础层**：完整 `m/p/s/z/f` 编码、108/136/144 牌墙、字牌/花牌/34 维胡牌计数、手牌与听牌基础兼容。
2. **规则策略接口**：`CapabilitySet`、`TileSetPolicy`、`OpeningPolicy`、`ClaimPolicy`、`SelfActionPolicy`、`WinPolicy`、`ScoringPolicy`、`TerminationPolicy`、`RuleStateCodec`、`RuleStateProjector`。
3. **room 主流程策略化**：动作窗口、开局流程、胡牌、杠、终局、结算与快照恢复只调用策略；持久化只接受当前通用 schema，旧快照字段不再迁移。
4. **四川血战策略包迁移**：换三张、定缺、无吃、缺门约束、血战续行、三家胡/牌墙空终局、查花猪、查大叫、退税保持用户可见兼容。
5. **国标 MCR 策略包**：144 张含花牌、吃碰杠、首胡终局、8 分起胡、81 fan registry/matcher/scorer 与结算流水。
6. **协议投影、CLI 与文档**：`ListRules` 暴露国标与川麻；开局提交统一为 `opening_action`，完成投影统一为 `opening_done`；CLI 支持字牌/花牌和服务端权威行动窗口；文档说明规则边界和能力开关。

## 验收矩阵

| 子系统 | 审查重点 | 主要测试 |
| --- | --- | --- |
| room 架构边界 | 非测试代码不 import 具体规则包，不出现规则私有字段读写或旧玩法主模型写出 | `TestRoomEngineArchitectureGuard`、`TestMarshalRoundPersistJSONDoesNotWriteLegacyRuleFields` |
| 策略可插拔 | 同一 room 管线支持首胡终局、血战续行、血流式续行、自定义开局和多响/头跳 fake rule | `TestRuleStrategyConformanceHuAftermathAndTermination`、`TestRuleStrategyConformanceMultiHuClaimWindow`、`TestRuleStrategyConformanceCustomOpeningProjection` |
| RuleState opaque | room 只保存/转交 `schema_version`/`data`，旧缺门/换三张内嵌字段不再解析 | `TestRuleStateMarshalsOnlyOpaqueFields`、`rule_state_test.go` |
| Opening 泛化 | common rules 不声明四川动作名；room 不保存 `ExchangeSubmitted`/`QueSubmitted`，只用 `OpeningPolicy.CurrentStep().Action` 和 `OpeningSubmittedByAction` | `TestCommonRulesDoNotDeclareSichuanOpeningActions`、`TestRoomEngineArchitectureGuard`、`TestOpeningLegacyProtocolDefinitionsRemoved` |
| 客户端 opening 展示 | CLI/bot/supervisor 不再建模 `PhaseExchange`/`PhaseQueMen` 或按四川 action 写通用调度分支；四川 action 只留在 adapter/策略层 | `TestOpeningClientRuntimeDoesNotModelSichuanPhases`、`cmd/cli`、`internal/bot`、`internal/app` |
| 生产规则能力 | 国标和川麻显式声明运行策略；`CapabilitiesOf` 缺能力即失败 | `TestProductionRulesDeclareExplicitRuntimeStrategies` |
| 川麻行为 | 换三张、定缺、缺门、血战续行、查花猪、查大叫、退税行为保持 | `internal/mahjong/sichuan/xuezhandaodi`、`internal/service/room` 回归 |
| 国标 MCR | 81 fan 必须有 `primary/rawWant/scoredWant` fixture；低于 8 分失败；无番和、花牌、自摸/点炮结算覆盖 | `TestMCRFanFixturesCoverEveryRegistryFan`、`TestMCRFanFixturesScoreAndSuppress`、`TestMCRRepresentativeFixtureExactItems`、`TestMCRChickenHandRequiresNoOtherFan`、`TestMCRSettlementPayments` |
| ListRules/CLI 投影 | 客户端只消费服务端规则元数据、行动窗口和结算投影，不本地推导玩法计分 | `TestListRulesReturnsRegisteredRuleMeta`、`TestLocalRoomGatewayListRules`、`cmd/cli/table_frontend_model_test.go` |

## 审查风险

- MCR scorer 已有 81 fan 覆盖与代表性精确分值测试，但完整 Green Book 的“不可拆移/一次计分”仍应在后续规则精度阶段继续补组合负例。
- `rules.Rule` 当前只承担注册元数据职责；规则包必须显式实现完整 `CapabilitySet`，不存在旧门面或 fallback 接入路径。
- `exchange_three` / `que_men` 仍作为四川规则动作名出现在规则包、CLI/bot adapter 和测试夹具中；它们不属于 common rules、room 主运行语义或客户端本地 phase。旧持久化字段已废弃，恢复路径直接拒绝。
- 本轮默认规则保持 `sichuan_xuezhandaodi_huansanzhang`，避免改变现有入口；国标只通过显式 `rule_id` 创建。

## 审查前命令

```bash
go test ./internal/service/room
go test ./internal/mahjong/rules
go test ./internal/mahjong/guobiao/jingji
GOCACHE=/private/tmp/lsp-server-go-cache go test ./...
```

如本地沙箱禁止 `httptest` 或临时服务端口绑定，用同一条全仓命令在非沙箱权限下复跑，并在审查说明中记录原因。
