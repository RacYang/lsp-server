---
description: 通过共享规则接口新增麻将变体实现。用于引入新规则集、变体专属计分或本地化玩法选项。
---

# 新增麻将规则

## When to use

当需要引入新的麻将变体、规则集本地化选项、专属计分模型，或把现有规则抽象为可插拔实现时使用。

## Inputs

- 规则名称与玩法差异：明确与既有规则的差异边界。
- ADR 背书：新规则集应先有独立 ADR 或明确产品决策。
- 规则 ID：必须符合 ADR-0041 的 `<region>_<variant>_<option>` 全拼音格式，禁止拼音首字母缩写和英文混写。
- 测试夹具：至少覆盖发牌、动作、和牌、结算四类关键路径。

## Steps

1. 在 `internal/mahjong/rules` 维持共享接口，并通过 `CapabilitySet` 组合开局、吃碰杠胡、轮转、计分、结算、终局与投影能力。
2. 在规则专属包中实现变体逻辑，保持麻将算法层与传输、会话、存储隔离；room engine 不得依赖具体规则包。
3. 如需注册表或配置选择，先把规则 ID 加入 `.build/config.yaml` 的 `mahjong.rules.allowed_ids`，再注册实现，并补配置解析与能力元数据测试。
4. 为新行为补充 YAML 夹具与 Go 单测，至少覆盖发牌、动作、和牌、结算、重连快照与多局边界。
5. 更新 `docs/RULE-ENGINE.md`、相关 ADR、`docs/PROTOCOL.md`、`docs/cli-tui-backend-gaps.md` 与 CHANGELOG；只有治理硬约束变更才更新 `docs/RULE-LIFECYCLE.md`。

## Verify

- 运行 `go test ./internal/mahjong/... ./internal/service/room/...`。
- 运行 `make verify-fast`；若新增协议或持久化字段，再运行 `make verify`。
