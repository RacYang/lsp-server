---
name: add-fan-type
description: 扩展现有麻将规则的计分逻辑。用于新增番种、结算分支或规则专属倍数。
---

# 新增番种

## When to use

当需要在现有麻将规则中新增番种、调整结算分支、增加规则专属倍数，或扩展 `ScoreContext` 消费逻辑时使用。

## Inputs

- 规则归属：优先确认是否属于 `internal/mahjong/sichuanxzdd` 当前规则。
- 计分语义：番种名称、倍数、触发条件、互斥或叠加关系。
- 测试样例：正例、反例，以及需要覆盖的边界局面。

## Steps

1. 只修改规则作用域内的计分包，不把番种判断放到传输层、存储层或 handler。
2. 如需新增上下文字段，先扩展 `internal/mahjong/rules` 的共享结构，再由房间引擎填充。
3. 在 `internal/mahjong/sichuanxzdd` 中实现番种判断，保持名称显式、可确定性推导。
4. 在 YAML 驱动测试或 Go 单测中同时补正例与反例。
5. 更新 `docs/MAHJONG-ALGORITHMS.md` 或相关 ADR/CHANGELOG 条目，说明新增语义。

## Verify

- 运行 `go test ./internal/mahjong/... ./internal/service/room/...`。
- 运行 `make verify-fast`；若结算事件或协议字段变化，再运行 `make verify`。
