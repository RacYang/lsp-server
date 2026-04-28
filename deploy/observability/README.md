# 可观测性规则

本目录收纳 Phase 6 SLO 所需的 Prometheus 规则。

- `recording-rules.yaml`：把 ADR-0019 的 `lsp_*` 指标整理为 SLO 口径。
- `alerting-rules.yaml`：每个 SLO 提供 `warn` 与 `page` 两级告警。

对外承诺的最小子集为抢答窗口完成率、重连成功率、结算持久化 p99；actor 队列、WebSocket 限流与未知消息率作为内部健康指标。

本地校验：

```bash
promtool check rules deploy/observability/*.yaml
```
