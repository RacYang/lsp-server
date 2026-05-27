---
description: 修改治理 SSOT。用于编辑 .build/config.yaml、同步 schema、派生产物、rules、负例与 verify 流水线。
---

# 更新 SSOT

## When to use

当需要改变治理阈值、工具版本、lint/arch/deps/proto/git/language/logging 等工程规则，或新增被 enforcer 读取的配置节时使用。

边界：改阈值、版本、白名单、列表项或既有配置结构时使用本命令；新增一条可执行规则维度时先使用 `/add-adr` 决策，再使用 `/add-constraint` 落地。

## Inputs

- SSOT 字段：`.build/config.yaml` 中的路径与新值。
- 消费者：`.build/derive.sh`、`scripts/verify-*.py|sh`、Makefile 或 Git hook。
- 负例：能证明新约束实际生效的最小样本。

## Steps

1. **输出职责所有者分析**：列出涉及该职责的现有文件、接口、调用链；写出目标状态与当前状态的 diff，确认落点在正确层（ADR-0049）。未完成此步骤不得动代码。
2. 先编辑 `.build/config.yaml`，不要直接改派生产物作为事实源。
3. 同步 `.build/schema/config.schema.json`，确保新字段被 schema 接纳。
4. 如字段参与派生，更新 `.build/derive.sh` 并运行 `make generate`。
5. 如字段参与校验，更新对应校验脚本、rule 与负例样本。
6. 确认 hook/CI 映射仍符合 `git.ci_parity`。

## Verify

- 运行 `make generate`。
- 运行 `make verify-config` 与 `make verify-determinism`。
- 运行 `make verify-meta`。
- 最后运行 `make verify`。
