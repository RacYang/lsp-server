---
description: 观测规则引用的存储操作标签须与代码中的实际调用一致
globs: ["deploy/observability/*.yaml"]
---

# 可观测规则一致性

`deploy/observability/` 下的 Prometheus 告警与记录规则中引用的 `store` 和 `op` 标签，必须与代码中 `ObserveStorage(store, op)` 调用一致。

新增存储操作时须同步检查观测规则是否需要更新对应标签维度。

---

- **ADR**：`docs/adr/0019-observability-metrics.md`
- **Enforcer**：`scripts/verify-observability.py`
- **负例**：`.build/negatives/metrics_bad_prefix.go.neg`
