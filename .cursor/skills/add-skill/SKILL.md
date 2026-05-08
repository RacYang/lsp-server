---
name: add-skill
description: 新增 Agent Skill。用于为新的任务类型增加标准流程并接入覆盖矩阵。
---

# 新增技能

## When to use

当出现重复任务流程，且 AGENTS 路由中缺少稳定入口时使用。

## Inputs

- skill 名称，使用小写短横线。
- 生命周期阶段与覆盖矩阵单元。
- 关联 ADR、rules 与 templates。

## Steps

1. 使用 `.cursor/templates/doc/skill/manifest.yaml` 对应骨架创建 SKILL.md。
2. 保持四个锚点：When to use、Inputs、Steps、Verify。
3. 更新 coverage matrix 与 AGENTS.md 任务路由。
4. 不在 skill 中复述 rule 的 enforcer 或负例细节。

## Verify

- 运行 `make verify-skeleton`。
- 运行 `make verify-doc-links`。
