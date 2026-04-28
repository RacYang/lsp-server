# ADR-0025 Phase 6 压测剧本与容量基线

## 状态

草案。等首轮压测完成、`runtime.*` 与 mailbox 容量基线取得实测数据后再升级为已采纳。

## 背景

ADR-0022 为运行时参数与存储弹性确立了默认值（`ws_rate_limit_per_second`、`ws_idempotency_cache`、`mailbox_capacity` 等），但默认值是工程直觉、不是来自实测。当前我们没有：

1. 可重复执行的压测剧本（脚本 / 仪表 / 抓取流程）。
2. 单进程与三进程拓扑下的吞吐 / 延迟 / 资源水位基线。
3. 把容量数据回写到 `runtime.*` 与 SLO 的闭环流程。

ADR-0023 已把"压测剧本与容量基线"划入 Phase 6 范围，本 ADR 给出剧本骨架、关键指标与回写路径。

## 决策

### 1. 压测剧本

#### 1.1 目标场景

按"用户可见关键路径"分三类：

- **Scenario A — 单房间稳态**：4 玩家、连续 100 局、抢答与碰杠按概率分布触发。验证 actor 事件循环吞吐、结算延迟。
- **Scenario B — 大会话压力**：N 个房间并发开局，观察 mailbox 水位、`lsp_actor_queue_depth` 分布、PostgreSQL `append_event` p99。
- **Scenario C — 重连冲击**：稳态运行中突然杀掉 50% gate 连接，观察 30 秒内重连恢复曲线。

每个 Scenario 都给出"通过条件"，把通过条件写到 `deploy/load/` 中的 YAML 剧本里。

#### 1.2 工具与脚本

- 客户端：基于 `nhooyr.io/websocket` 写最小压测客户端（Go），按 Scenario 加载牌谱与动作脚本。
- 调度：从一台单独的"打手机"运行；不在被测服务自身节点上跑压测客户端，避免自相耗资源。
- 数据仓：把 Prometheus 在压测窗口的 metric 快照导出到 `bench/<run_id>/metrics.json`，便于离线对比。
- 用例最小集合：先覆盖 Scenario A，再扩到 B / C。

每个 Scenario 必须能用 `make verify-bench`（新增 phony target）触发，但不在默认 `make verify` 中跑。

#### 1.3 输出格式

每次压测产出：

```text
bench/<run_id>/
  config.yaml          # 入参：goroutine 数、房间数、规则集、版本号
  metrics.json         # 关键指标快照
  flamegraph.svg       # 可选：CPU 火焰图
  summary.md           # 人类可读结论
```

`summary.md` 必须给出"是否通过通过条件"与"建议的 `runtime.*` 调整方向"。

### 2. 容量基线维度

每条压测必须采集：

| 维度 | 指标 |
| --- | --- |
| 吞吐 | 每秒成局、每秒结算 |
| 延迟 | 抢答窗口 p50/p95/p99；结算端到端 p99 |
| 队列水位 | `lsp_actor_queue_depth`（每房间） |
| 限流 | `lsp_rate_limited_total{layer="ws"}` 速率 |
| 资源 | gate / room 进程的 CPU、RSS、goroutine 数 |
| 存储 | `lsp_storage_op_seconds{store,op,result}` p50/p99 |

第一轮基线建议在以下硬件上跑：

- 4 vCPU / 8 GiB 物理机或等价云主机一台跑被测服务。
- 至少一组（Redis + PostgreSQL + etcd）依赖项。

### 3. 回写路径

压测出基线后，按以下规则回写：

1. 把推荐的 `runtime.*` 数值变更落地到 `internal/config` 与示例 YAML，不修改 `.build/config.yaml`。
2. ADR-0024 SLO 表中的目标值 / 告警阈值若与实测脱节，立 follow-up commit 调整 ADR-0024（保留历史值便于对比）。
3. 若 mailbox 实际水位长期超过 `runtime.room.mailbox_capacity * 0.8`，必须在容量 ADR follow-up 中讨论是否扩容或拆分房间分片。

### 4. 工程边界

- 本 ADR 不引入新运行时参数；只决定"何时基于实测调整既有参数"。
- 本 ADR 不强制把压测接入 CI；CI 只跑 `make verify`，压测是独立流水线。
- 本 ADR 不评估第二套规则集的容量，只确保剧本本身具备 plug-in 能力（通过 `Rule` 接口替换）。

## 后果

- ADR 通过后，Phase 6 实施计划包含：压测客户端骨架、Scenario A 用例、`bench/` 输出格式约定、`make verify-bench` 占位。
- 首轮基线出来后 ADR-0024 SLO 表与 `runtime.*` 默认值会同步更新，并把本 ADR 状态升级为"已采纳"。
- 引入新场景（如新规则集、跨地域）时需补独立 ADR，不复用本 ADR 的剧本约束。
