# ADR-0022 Phase 5.5 运行时参数与存储弹性

## 状态

已采纳。

## 背景

Phase 5 已引入 WebSocket 限流、幂等重放去重、room actor 有界 mailbox 与 Redis 幂等 TTL，但这些默认值原先散落在代码中，生产容量调整需要重新编译。存储层也缺少统一的超时、重试与错误分类口径。

## 决策

### 1. 运行时参数

运行时参数由进程 YAML 与 `internal/config.Config` 管理，不从 `.build/config.yaml` 派生。`.build/config.yaml` 继续作为治理、lint、hook 与覆盖率门槛的 SSOT。

以下参数可通过 `runtime.*` 配置覆盖，默认值保持 Phase 5 行为不变：

- `runtime.gate.ws_rate_limit_per_second`: 20。
- `runtime.gate.ws_rate_limit_burst`: 40。
- `runtime.gate.ws_idempotency_cache`: 4096。
- `runtime.room.mailbox_capacity`: 96（Phase 6 首轮本地基线后由 64 保守上调）。
- `runtime.redis.idempotency_ttl`: 10 分钟。

### 2. 存储弹性

Redis 与 PostgreSQL store 层统一使用可复用 helper 标注可重试错误、包裹操作超时并在幂等操作上执行有限退避重试。非幂等写入只增加超时与错误分类，不自动重试。

### 3. 指标兼容

`lsp_storage_op_seconds{store,op,result}` 保留 `ok/error` 语义，避免破坏既有大盘。重试次数使用新增 counter 表达，避免把现有 `result="error"` 静默拆分成多个值。

## 后果

- 配置示例与 `internal/config` 必须同步维护。
- 新增运行时参数时优先在单元测试覆盖默认值与 override 行为。
- 后续若将无 Docker 集成目标接入 CI，应显式更新 workflow 与 hook/CI 映射说明。
