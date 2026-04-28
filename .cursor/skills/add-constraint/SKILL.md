---
name: add-constraint
description: 端到端新增仓库硬约束。用于引入新的工程硬规则、enforcer 或负例并接入治理流水线。
---

# 新增约束

## When to use

当需要新增工程硬规则、扩展现有 enforcer、接入新的负例，或把 ADR 中的硬约束升级为自动校验时使用。

## Inputs

- 约束来源：已有 ADR，或需要新增的 ADR。
- SSOT 字段：优先落在 `.build/config.yaml`，必要时同步 `.build/schema/config.schema.json`。
- 负例意图：必须能稳定触发对应 enforcer 失败。

## Steps

1. 判断是否已有 ADR 背书；没有则先新增 ADR，不要只在 rule 中发明规则。
2. 如需配置，先编辑 `.build/config.yaml`，并同步 `.build/schema/config.schema.json`。
3. 新增或更新 `scripts/verify-*.py` / `scripts/verify-*.sh`，保持脚本可单独运行。
4. 在 `.build/negatives/` 增加最小隔离负例，命名需能表达失败原因。
5. 新增或更新 `.cursor/rules/*.mdc`，保持 `kind` / `adr` / `enforcer` / `negative_test` 三元组一致。
6. 如新增负例类型，扩展 `scripts/verify-negatives.sh` 的分派逻辑。

## Verify

- 运行 `make generate`（如改动 `.build/config.yaml` 或派生产物）。
- 运行 `make verify-meta`，确认 rule frontmatter 与负例闭环通过。
- 运行包含新 enforcer 的目标，最后运行 `make verify`。
