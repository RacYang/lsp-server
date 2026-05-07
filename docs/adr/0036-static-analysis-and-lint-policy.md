---
title: 静态分析与 nolint 治理策略
status: accepted
date: 2026-05-07
---

# ADR-0036 静态分析与 nolint 治理策略

## 状态

已采纳。

## 背景

CI 曾因 workflow 触发、工具版本与本地 bootstrap 探测不一致而长期漏跑，升级 `golangci-lint` 后一次性暴露出较多历史 lint 债。若继续在调用点追加 `nolint`，会让真实设计边界、工具误报与临时压制混在一起，后续每次升级都难以判断哪些豁免仍然成立。

本仓库的治理原则是 SSOT 优先、派生产物可复现、负例可执行。因此静态分析配置也应由 `.build/config.yaml` 统一派生，常见误报用全局规则表达，行级豁免只保留给无法抽象到 SSOT 的窄场景。

## 决策

1. `golangci-lint` 的启用 linter、linter settings 与 exclusions 均从 `.build/config.yaml` 派生到 `.golangci.yml`。
2. 常见且稳定的误报必须优先落到 SSOT exclusions 或 linter settings，不在业务代码里批量保留行级 `nolint`。
3. 行级 `nolint` 必须满足三项条件：显式指定已启用 linter、在同一行写明原因、原因描述具体到当前不变量或 ADR。
4. 禁止为未启用 linter 保留 `nolint`，这类标注不能产生校验效果，只会制造噪声。
5. `scripts/verify-nolint-policy.py` 作为硬约束接入 `verify` 与 `verify-fast`，并通过 `.build/negatives/nolint_policy_*.go.neg` 覆盖裸 `nolint`、缺少原因、未启用 linter 三类负例。

## 后果

新增或修改 `nolint` 时，维护者需要先判断是否能由 SSOT 全局规则覆盖；只有确实依赖局部不变量时才允许行级豁免。工具升级带来的新增误报应优先沉淀为配置或类型约束，而不是分散压制。短期内这会让少数保留豁免更显式，但长期能降低 lint 配置、代码注释与 CI 行为之间的漂移。
