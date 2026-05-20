---
description: 治理工具版本须锁定在 SSOT，偏差由 verify-tools 检测
alwaysApply: true
---

# 工具版本锁定

- `go`、`golangci-lint`、`buf`、`yq`、`yamllint`、`markdownlint-cli2`、`shellcheck` 等治理工具的版本锁定在 `.build/config.yaml` 的 `tools` 节。
- `make verify-tools` 对比已安装版本与 SSOT 锁定的版本，偏差时报错。
- 升级工具版本须通过 `/update-ssot` 修改 SSOT，不得直接改派生文件或本地环境。

---

- **ADR**：`docs/adr/0000-engineering-charter.md`
- **Enforcer**：`Makefile#{verify-tools}`
- **负例**：`.build/negatives/config_schema_missing_key.yaml.neg`
