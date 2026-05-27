---
description: 高频运行态指标候选不得作为日志字段
globs: ["internal/**/*.go", "cmd/**/*.go", "pkg/**/*.go"]
---

# 日志与指标边界

`qps`、`mailbox_depth` 等高频运行态字段应进入 Prometheus 指标，不应写入业务日志。

---

- **ADR**：`docs/adr/0006-logging-system-and-facade.md`
- **Enforcer**：`scripts/verify-log-calls.py`
- **负例**：`.build/negatives/lang_log_metric_like_key.go.neg`
