# bench

Phase 6 压测剧本、运行脚本与基线记录。

## 命名约定

- `scenario_a/`、`scenario_b/`、`scenario_c/`：压测输入剧本，目录名使用 `snake_case`，与 `cmd/loadgen/scenario_a.go` 等 Go 文件保持一一映射。
- `phase6-preprod-YYYYMMDD/`：预发压测基线归档，目录名使用 `kebab-case` 与 8 位日期，供 ADR 和变更日志稳定引用。
- `restore-phase6-preprod-YYYYMMDD/`：恢复演练摘要归档，命名口径同压测基线。
- `YYYYMMDDTHHMMSSZ-${scenario}/`：`bench/scripts/run.sh` 默认生成的临时输出目录，不进入 Git，用后清理。
- `scripts/`：压测入口脚本目录。

## 入口

```bash
SCENARIO=a make verify-bench
```

默认场景为 `a`；可通过 `CONFIG`、`RUN_ID` 与 `OUT_DIR` 覆盖配置路径、执行编号和输出目录。
