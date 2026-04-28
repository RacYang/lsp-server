# 测试

## 层次

1. 针对确定性包的单元测试。
2. 针对麻将示例的夹具测试。
3. 针对房间编排的服务测试。
4. 针对单进程 MVP 的端到端测试。
5. 针对重连、幂等、限流与托管超时的 integration 目标。
6. 针对 Phase 6 容量假设的压测剧本与基线记录。

## 覆盖率策略

覆盖率阈值在 `.build/config.yaml` 中配置。生成代码与非关键编排路径按策略排除。

## 负例样本

每条约束 enforcer 须有隔离的负例，存放在 `.build/negatives`。

## Integration

`RUN_INTEGRATION=1 make verify-test-integration` 当前覆盖：

- 集群重连会话恢复。
- WS `idempotency_key` 重放去重。
- room actor mailbox 满队列限流。
- 基于 `fakeClock` 的托管超时推进。

进程级重启恢复回放仍保留为专项测试，默认 integration 目标优先选择稳定且耗时可控的链路。

## 压测

`cmd/loadgen` 与 `bench/scenario_*` 提供 Phase 6 压测入口，默认不进入 `make verify`：

- `SCENARIO=a make verify-bench`：单房间稳态。
- `SCENARIO=b make verify-bench`：大会话压力。
- `SCENARIO=c make verify-bench`：重连冲击。

预发基线结果归档在 `bench/phase6-preprod-20260428`；容量与 SLO 回写口径见 [ADR-0025](adr/0025-load-and-capacity.md)。
