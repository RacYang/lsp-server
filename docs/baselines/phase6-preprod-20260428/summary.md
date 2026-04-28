# Phase 6 预发压测基线

## 结论

- 执行环境：预发。
- 执行编号：`phase6-preprod-20260428`。
- 执行范围：`scenario_a`、`scenario_b`、`scenario_c` 各连续 3 轮。
- 结论：三类场景均通过首轮通过条件，ADR-0024 对外 SLO 与 ADR-0025 容量回写路径可进入 follow-up 收口。

## 场景结果

| 场景 | 通过轮数 | 关键观察 | 结论 |
| --- | --- | --- | --- |
| A 单房间稳态 | 3 / 3 | 抢答完成率最低 99.3%，PostgreSQL `append_event` p99 最高 173ms | 通过 |
| B 大会话压力 | 3 / 3 | mailbox p95 最高为容量 63%，PostgreSQL `append_event` p99 最高 188ms | 通过 |
| C 重连冲击 | 3 / 3 | 30 秒内重连成功率最低 99.5%，mailbox p95 最高为容量 51% | 通过 |

## SLO 回写

- 抢答窗口完成率：保持 90 天 `result="completed"` / 总和 ≥ 99%。
- 重连成功率：保持 30 天 `result="ok"` ≥ 99%。
- 结算延迟 p99：保持 `lsp_storage_op_seconds{store="postgres",op="append_event"}` p99 ≤ 200ms。
- 告警阈值沿用 ADR-0024 首轮表格；线上运行 1～2 周后若与真实负载脱节，再以独立 follow-up 修订。

## runtime.* 建议

- `runtime.room.mailbox_capacity` 保持 96；本次 `scenario_b` mailbox p95 最高为容量 63%，未触发 ADR-0025 的 80% 讨论线。
- `runtime.gate.ws_rate_limit_per_second` 保持 20，`runtime.gate.ws_rate_limit_burst` 保持 40。
- 不新增 `runtime.*`，不修改 `.build/config.yaml`。

## 后续观察

- 将本执行编号写入 ADR-0024 与 ADR-0025 状态段落。
- 若生产真实流量中 mailbox p95 连续超过容量 80%，必须按 ADR-0025 新增容量 follow-up，而不是直接改默认值。
