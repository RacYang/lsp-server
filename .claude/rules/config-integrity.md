---
description: SSOT 配置文件须通过自身 schema 校验
alwaysApply: true
---

# 配置完整性

`.build/config.yaml` 是仓库唯一人工编写的治理事实源，其内容必须通过 `.build/schema/config.schema.json` 的结构化校验。

- 新增或修改 SSOT 字段时，必须同步更新对应的 schema 文件。
- 派生配置（`.golangci.yml`、`.go-arch-lint.yml`、`.commitlintrc.json` 等）由 `make generate` 从 SSOT 派生，不得手改。

---

- **ADR**：`docs/adr/0000-engineering-charter.md`
- **Enforcer**：`scripts/verify-config-schema.py`
- **负例**：`.build/negatives/config_schema_missing_key.yaml.neg`
