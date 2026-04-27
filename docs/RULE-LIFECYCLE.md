# 规则生命周期

规则用于引导人类编辑与 AI 辅助。**Constraint** 规则属于仓库契约的一部分，且必须始终可执行。

## 生命周期

```mermaid
flowchart LR
    idea["提案：新约束"] --> charterCheck{"初始基线规则？"}
    charterCheck -->|"是"| charter["引用 ADR-0000"]
    charterCheck -->|"否"| adr["撰写独立 ADR"]
    charter --> sot["编辑 .build/config.yaml"]
    adr --> sot
    sot --> gen["运行 make generate"]
    gen --> neg["新增 .build/negatives 负例"]
    neg --> verify["运行 make verify"]
    verify --> rule["创建 .cursor/rules/*.mdc"]
    rule --> merge["评审后合并"]
```

## 必填字段

- `constraint`：`adr`、`enforcer`、`negative_test`
- `norm`：`adr`

## 验收

一条规则仅在以下条件满足时被接受：

1. 元数据符合 `.build/schema/rule.schema.json`。
2. enforcer 已接入 `make verify`。
3. 负例在隔离运行时按预期失败。

## 运行时 ADR

非治理约束但会改变协议、房间引擎、计时器、恢复或可观测性边界的变更，也应记录独立 ADR。Phase 5 示例包括 proto baseline 重置、room 引擎与结算边界、时钟/托管调度、可观测指标最小集合。
