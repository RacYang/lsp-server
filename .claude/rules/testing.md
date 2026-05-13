---
description: 测试代码要通过 testifylint 并满足覆盖率门槛
globs: ["**/*_test.go"]
---

# 测试

- 倾向表驱动测试与明确期望。
- 麻将算法变更应新增或更新 YAML 夹具。
- 覆盖率门槛由 `.build/config.yaml` 的 `coverage.thresholds` 节定义，不得降低。

---

- **ADR**：`docs/adr/0000-engineering-charter.md`
- **Enforcer**：`.golangci.yml#{testifylint}`
- **负例**：`.build/negatives/testing_bad_assertion.go.neg`
