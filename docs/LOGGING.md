# 日志体系

本文档落实 [ADR-0006](adr/0006-logging-system-and-facade.md)、[ADR-0033](adr/0033-logging-sampling-and-pii-redaction.md)、[ADR-0034](adr/0034-logging-dynamic-level-control.md) 与 [ADR-0035](adr/0035-otel-logs-bridge.md)，并以 `.build/config.yaml` 的 `logging` 节为事实源。

## 统一门面

- 模块路径：`racoo.cn/lsp/pkg/logx`。
- 业务、领域、handler、service 等代码仅通过该门面写日志。
- 禁止在豁免路径之外 `import` 或使用 `log/slog`、`go.uber.org/zap`、`go.uber.org/zap/zapcore`。
- `pkg/logx/**` 是底层日志实现的唯一豁免区。

## Context 注入

`trace_id`、`user_id` 与 `room_id` 不再由每条日志调用手写，而是在边界处写入 `context.Context`，由门面 Handler 自动注入。

```go
ctx = logx.WithTraceID(ctx, tid)
ctx = logx.WithUserID(ctx, uid)
ctx = logx.WithRoomID(ctx, rid)
logx.Info(ctx, "玩家进入房间", "rule_id", ruleID)
```

`verify-log-calls.py` 会拦截业务调用现场手写 `trace_id`、`user_id`、`room_id` 的行为；测试文件与显式桥接文件可豁免。

## 字段命名

结构化字段采用两层治理：

- 强约束：字段名必须匹配 `logging.field_naming.pattern`，当前为英文 `snake_case`，长度不超过 32。
- 核心字典：`trace_id`、`span_id`、`user_id`、`room_id`、`rule_id`、`op`、`err`、`elapsed_ms`、`shard`、`region` 等跨服务字段必须全局同名。

业务私有字段无需先进入 SSOT，但仍必须满足命名规则。

## 敏感字段

`token`、`password`、`secret`、`mobile`、`email`、`ip` 等键不得在业务日志调用中直接出现。Phase 1 仅做键名层治理与输出脱敏；值层正则、嵌套结构遍历与更强 PII 识别留给后续 ADR。

## 采样与级别

- `Debug`：开发期诊断，默认可关闭。
- `Info`：关键业务里程碑、可审计路径。
- `Warn`：可恢复问题、降级、重试。
- `Error`：需关注且可能影响用户体验或数据一致性。

采样框架默认关闭；即使未来启用，`Error` 级日志也不得被采样丢弃。动态级别 Phase 1 仅提供 `logx.AtomicLevel` Go API，不开放 HTTP 写端点。

## 与指标边界

限流、幂等重放、抢答窗口、自动托管、重连结果、actor 队列深度与存储耗时以 Prometheus 指标为主。`qps`、`mailbox_depth`、`active_connections` 等高频运行态字段不得写入日志。

## 测试

业务测试优先使用 `logx.NewObserver()` 捕获日志，避免手写 `bytes.Buffer` 与 JSON 解析。Observer 会同样走 Context 自动注入，能作为业务代码的参考样板。

## 与 CI 的关系

- `verify-no-direct-logging.py`：拦截非法直调底层日志包。
- `verify-log-calls.py`：校验中文 message、字段名、敏感键、指标候选键与上下文字段字面量。
- `verify-log-boundaries.py`：校验已存在的边界文件是否注入日志上下文字段。
