---
description: 更新 Protobuf 兼容性基线。用于协议兼容窗口完成后的 proto-baseline 标签演进。
---

# 更新 Proto 基线

## When to use

当协议演进已经完成兼容性评估，需要移动 `proto-baseline` 对照基准时使用。

## Inputs

- 新基线提交。
- breaking 检查结果。
- 关联 ADR 或协议说明。

## Steps

1. **输出职责所有者分析**：列出涉及该职责的现有文件、接口、调用链；写出目标状态与当前状态的 diff，确认落点在正确层（ADR-0049）。未完成此步骤不得动代码。
2. 确认当前 proto 变更只追加字段或已有独立兼容性决策。
3. 运行 buf lint 与 breaking 检查。
4. 更新或移动基线标签前，确认维护者已授权。
5. 同步协议文档与 ADR 索引。

## Verify

- 运行 `make verify-proto`。
- 运行 `make verify-proto-break`。
