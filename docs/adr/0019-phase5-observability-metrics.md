# ADR-0019 Phase 5 可观测性指标最小集合

## 状态

Accepted

## 背景

Phase 5 引入血战完整化、actor 有界队列、自动托管与重连恢复后，排障重点从「进程是否存活」转向「房间是否被队列压住、抢答窗口是否持续打开、恢复链路是否异常、存储是否成为尾延迟来源」。

## 决策

先落地最小可决策指标集合：

- `lsp_claim_window_total{result}`：抢答窗口打开及最终动作（胡/杠/碰）计数。
- `lsp_auto_timeout_total{kind}`：按等待态记录托管触发次数。
- `lsp_reconnect_total{result}`：记录恢复成功、重定向与失败。
- `lsp_actor_queue_depth{room}`：暴露房间 actor mailbox 当前深度。
- `lsp_storage_op_seconds{store,op,result}`：记录 Redis/PostgreSQL 关键操作耗时，用 histogram 支持 p99 观察。
- `lsp_rate_limited_total{layer}`、`lsp_idempotent_replay_total`、`lsp_unknown_msg_total`：接通 Phase 5.5 限流、幂等与未知消息观测。

`/metrics` 仍使用 Prometheus 默认 registry，并在 obs 启动时显式确保 Go collector 与 Process collector 注册。

## 后果

该集合优先覆盖线上判断所需的高信号指标，避免在 SLO 未定义前铺开大量低价值 label。后续只有当 SLO 或告警规则需要时，再增量引入更细的番种、房间状态与节点路由指标。
