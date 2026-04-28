# bench

Phase 6 压测剧本与运行脚本。本目录只保留**输入**（剧本与脚本）与**临时产物**（`runs/`），不再托管长期归档。

## 目录约定

- `scenarios/scenario_{a,b,c}/`：压测输入剧本，目录名使用 `snake_case`，与 `cmd/loadgen/scenario_a.go` 等 Go 文件保持一一映射。
- `scripts/`：压测入口脚本目录。
- `runs/<run_id>/`：`bench/scripts/run.sh` 的默认输出目录，`run_id` 形如 `YYYYMMDDTHHMMSSZ-${scenario}`，整体不进入 Git，用后清理。

长期引用的归档物落在 `docs/`：

- `docs/baselines/phase6-preprod-YYYYMMDD/`：预发压测基线归档，供 ADR-0024 / ADR-0025 与变更日志稳定引用。
- `docs/drills/phase6-preprod-YYYYMMDD/`：备份恢复演练摘要归档（沿用 ADR-0026 模板，不再重复 `restore-` 前缀）。

## 入口

```bash
SCENARIO=a make verify-bench
```

默认场景为 `a`；可通过 `CONFIG`、`RUN_ID` 与 `OUT_DIR` 覆盖配置路径、执行编号和输出目录。
