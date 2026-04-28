---
name: update-ssot
description: 修改治理 SSOT。用于编辑 .build/config.yaml、同步 schema、派生产物、rules、负例与 verify 流水线。
---

# 更新 SSOT

## When to use

当需要改变治理阈值、工具版本、lint/arch/deps/proto/git/language/logging 等工程规则，或新增被 enforcer 读取的配置节时使用。

## Inputs

- SSOT 字段：`.build/config.yaml` 中的路径与新值。
- 消费者：`.build/derive.sh`、`scripts/verify-*.py|sh`、Makefile 或 Git hook。
- 负例：能证明新约束实际生效的最小样本。

## Steps

1. 先编辑 `.build/config.yaml`，不要直接改派生产物作为事实源。
2. 同步 `.build/schema/config.schema.json`，确保新字段被 schema 接纳。
3. 如字段参与派生，更新 `.build/derive.sh` 并运行 `make generate`。
4. 如字段参与校验，更新对应 `scripts/verify-*`、`.cursor/rules/*.mdc` 与 `.build/negatives/*.neg`。
5. 确认 hook/CI 映射仍符合 `git.ci_parity`。

## Verify

- 运行 `make generate`。
- 运行 `make verify-config` 与 `make verify-determinism`。
- 运行 `make verify-meta`。
- 最后运行 `make verify`。
