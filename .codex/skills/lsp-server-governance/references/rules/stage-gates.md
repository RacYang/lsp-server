---
description: 项目阶段演进须通过 SSOT stage 与 stage_gates 显式管理
alwaysApply: true
---

# 阶段门控

- 当前项目阶段由 `.build/config.yaml` 的 `stage` 字段定义（`alpha`、`beta`、`ga`）。
- 每个阶段的 `cover_strict` 决定校验失败是否阻断流水线：`false` 时仅警告，`true` 时硬阻断。
- 阶段推进时必须同步更新 `stage` 值与对应的 `stage_gates` 配置，不得绕过门控直接修改 `cover_strict`。

---

- **ADR**：`docs/adr/0000-engineering-charter.md`
- **Enforcer**：`scripts/verify-config-schema.py`
- **负例**：`.build/negatives/config_schema_missing_key.yaml.neg`
