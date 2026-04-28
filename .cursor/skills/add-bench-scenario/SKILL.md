---
name: add-bench-scenario
description: 新增压测场景。用于扩展 bench/scenarios、cmd/loadgen 场景实现、verify-bench 入口与长期基线归档。
---

# 新增压测场景

## When to use

当需要新增 Phase 6 容量场景、预发压测剧本、重连冲击模型或新的可重复 loadgen 运行路径时使用。

## Inputs

- 场景代号：使用小写字母或 snake_case，与 `scenario_*` 文件一致。
- 通过条件：房间数、玩家数、轮数、SLO 观察项。
- 归档策略：临时产物在 `bench/runs/`，长期基线放 `docs/baselines/`。

## Steps

1. 在 `bench/scenarios/scenario_X/config.yaml` 新增剧本配置。
2. 在 `cmd/loadgen/scenario_X.go` 增加场景实现，保持命名与配置路径一一对应。
3. 更新 `bench/scripts/run.sh` 或 loadgen 分派逻辑，支持 `SCENARIO=X make verify-bench`。
4. 如形成长期基线，归档到 `docs/baselines/<run_id>/`，不要提交 `bench/runs/`。
5. 更新 `bench/README.md`、`docs/TESTING.md` 与 CHANGELOG。

## Verify

- 运行 `SCENARIO=X make verify-bench`。
- 运行 `make verify-fast`。
