---
name: add-mahjong-rule
description: 通过共享规则接口新增麻将变体实现。用于引入新规则集、变体专属计分或本地化玩法选项。
---

# 新增麻将规则

## When to use

当需要引入新的麻将变体、规则集本地化选项、专属计分模型，或把现有规则抽象为可插拔实现时使用。

## Inputs

- 规则名称与玩法差异：明确与既有规则的差异边界。
- ADR 背书：新规则集应先有独立 ADR 或明确产品决策。
- 测试夹具：至少覆盖发牌、动作、和牌、结算四类关键路径。

## Steps

1. 在 `internal/mahjong/rules` 维持共享接口，不让房间代码依赖具体规则实现。
2. 在规则专属包中实现变体逻辑，保持麻将算法层与传输、会话、存储隔离。
3. 如需注册表或配置选择，新增清晰的规则 ID，并补配置解析测试。
4. 为新行为补充 YAML 夹具与 Go 单测，确保夹具确定性与可回放。
5. 更新 `docs/RULE-ENGINE.md`、`docs/RULE-LIFECYCLE.md` 与 CHANGELOG。

## Verify

- 运行 `go test ./internal/mahjong/... ./internal/service/room/...`。
- 运行 `make verify-fast`；若新增协议或持久化字段，再运行 `make verify`。
