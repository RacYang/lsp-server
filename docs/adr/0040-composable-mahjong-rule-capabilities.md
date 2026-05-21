---
title: 组合式麻将规则能力
status: accepted
date: 2026-05-08
---

# ADR-0040 组合式麻将规则能力

## 状态

已采纳。

## 背景

ADR-0002 只规定房间代码依赖规则接口，不依赖具体变体包；ADR-0017 划分房间引擎与结算边界；ADR-0039 固定四川血战局内权威契约。实际实现中，`rules.Rule` 仍只覆盖牌墙、和牌、番种与终局判断，换三张、定缺、吃碰杠胡候选、血战续行与结算流水仍散落在 room engine 中。

这会导致新玩法只能复制或分支改造房间引擎。血流、无换三张、无定缺、允许吃牌、补杠抢杠等差异，本质上是细分规则能力的组合，不应成为传输层或房间 actor 的硬编码。

## 决策

1. `rules.Rule` 只保留注册元数据，运行能力由 `RuleCapabilitiesProvider` 返回的能力集合提供。房间运行路径必须使用能力集合，不存在 `BuildWall/CheckHu/ScoreFans/GameOver` 旧门面或 fallback 接入方式。能力集合至少描述规则元数据、牌组、开局流程、抢答动作、自动作、胡牌、轮转流程、计分、结算、终局与客户端投影。
2. 吃、碰、直杠、暗杠、补杠、抢杠胡、胡牌都视为可组合动作颗粒，由 `ClaimPolicy` 给出候选、优先级、互斥关系与展示动作名。room engine 只负责执行串行化后的权威动作；四川血战默认 `NoEatingClaimPolicy` 不产生吃牌候选，但 `ChiRequest/ChiResponse`、cluster `ChiEvent` 与 `TableGateway.Chi` 链路必须保持可用。
3. 换三张、定缺、选庄等开局步骤由 `OpeningPolicy` 描述，房间状态只保存通用等待态与规则运行态，不再把某个玩法的固定阶段当作所有规则的事实。开局完成通知由规则策略产出通用投影结果，room 只负责发送。
4. 摸牌、自摸窗口、杠后补牌、海底处理与已胡座位是否继续参与轮转由 `TurnFlow` 或等价能力描述，不再由血战实现的布尔字段隐式决定。
5. 计分与结算拆分为 `ScoringPolicy` 与 `SettlementPolicy`。房间层保存 `WinEvent` 与 `ScoreEvent`，规则包负责把事件折叠为可读结算、番种分解与累计积分。
6. 规则能力必须是确定性逻辑，不依赖传输、会话、存储、集群或 app 包。跨进程恢复只通过 `rule_id`、规则 schema、通用局面字段与 opaque `rule_state`。规则包通过 `RuleStateCodec` encode/decode 当前私有事实，通过 `RuleStateProjector` 输出协议投影；room 不得读取规则私有字段。
7. 对外协议只追加字段。结构化副露、最后动作、deadline、权威剩牌数、座位状态、局号、积分和规则元数据都由权威局面投影生成，handler 不拼业务事实。

## 后果

- 新增玩法应通过组合能力、规则 ID、测试夹具和文档注册进入系统，不复制房间引擎。
- 生产规则必须显式声明运行所需策略；`CapabilitiesOf` 不补任何运行策略，未声明完整能力的规则不能进入 room/lobby 运行路径。
- 现有四川血战实现先作为组合能力的第一条适配路径；`exchange_three`、`que_men` 等协议动作名可以保留，但不得重新成为 room 内部玩法分支。
- 完整能力开关必须由规则包测试守住。国标 `mcr_81_fans` 只有在 81 fan registry、主目标 fixture、raw/scored 断言、排除关系和全仓回归通过时才可暴露。
- `round_json.schema_version` 需要随规则运行态扩展递增。当前实现已硬切：只接受当前 schema，旧快照不迁移；新快照不得写出 `que_by_seat`、`hued_seats`、`winner_seats`、`ledger`、`total_fan_by_seat`、`exchange_tiles`、`exchange_done`、`que_done`、`exchange_dir` 等废弃主字段。
- `RuleState` 新写出只包含 `schema_version` / `data`；旧内嵌字段会被反序列化拒绝，不再作为 room 可读模型。
- CLI TUI A/B 类契约成为房间权威投影的验收标准，不能通过客户端估算或 handler 补字段替代。
