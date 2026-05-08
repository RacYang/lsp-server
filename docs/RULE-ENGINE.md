# 规则引擎

## 目标

麻将变体规则必须可替换，而无需重写传输、存储或房间编排。

## 设计

规则接口分为门面与能力集合。`rules.Rule` 仍是注册表入口，负责牌墙、和牌、番种与终局判断；`Rule.Capabilities()` 通过组合能力描述细分规则颗粒：

- 牌墙构建
- `OpeningFlow`：换三张、定缺、选庄等开局子流程。
- `ClaimPolicy`：吃、碰、明杠、暗杠、补杠、抢杠胡、胡等动作颗粒的可用性、互斥与优先级。
- `TurnFlow`：摸牌、自摸窗口、杠后补牌、海底处理与已胡座位轮转。
- `ScoringPolicy` / `SettlementPolicy`：番种上下文、流水折叠、罚分、退税与累计积分。
- `TerminationPolicy`：三家胡、牌墙耗尽、血流等终局条件。
- `RoundProjection`：TUI/重连所需的结构化副露、最后动作、deadline、剩牌数与规则元数据。

组合边界见 [ADR-0040](adr/0040-composable-mahjong-rule-capabilities.md)。ECS/组合模式只作为拆分职责的参考；仓库不引入通用游戏框架。

## 注册表

变体按名称注册并通过配置选择。房间通过**接口**请求规则行为，而非按规则名分支。

## 首个变体

`sichuan_xzdd` 为首个实现，支持：

- 定缺
- 无吃牌
- 和牌后血战续行
- 自摸、点炮胡与抢杠胡上下文
- 基础番种、海底/杠上/根与结构化结算

房间编排负责 actor 串行化、等待态、托管超时与持久化恢复；规则包不得依赖传输层、存储层或 room service。

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
