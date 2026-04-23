---
name: add-constraint
description: 端到端新增仓库硬约束。用于引入新的工程硬规则、enforcer 或负例并接入治理流水线。
---

# 新增约束

1. 若变更不属于宪章背书的初始规则集，须撰写独立 ADR。
2. 扩展 `.build/config.yaml`。
3. 更新派生或验证脚本。
4. 在 `.build/negatives` 下新增隔离负例。
5. 新增或更新对应的 `.cursor/rules` 文件。
6. 运行验证并确认负例按预期原因失败。
