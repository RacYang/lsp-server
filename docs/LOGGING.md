# 日志体系

本文档落实 [ADR-0006](adr/0006-logging-system-and-facade.md)，与 SSOT `.build/config.yaml` 的 `logging` 节一致。

## 统一门面

- 模块路径：`racoo.cn/lsp/pkg/logx`。
- 业务、领域、handler、service 等代码**仅**通过该门面写日志。
- **禁止**在豁免路径之外 `import` 或使用 `log/slog`、`go.uber.org/zap`、`go.uber.org/zap/zapcore`。

## 门面实现位置

- 允许在 `pkg/logx/**` 内封装底层实现；该目录为直调底层 logger 的**唯一**豁免区（由 `verify-no-direct-logging.py` 识别）。

## 建议 API 形态（Phase 1 实现）

以下仅为约定示例，非强制编译接口名：

```go
// 示例：门面在 Phase 1 提供类似签名（以实际代码为准）。
logx.Info(ctx, "玩家进入房间", "trace_id", tid, "user_id", uid, "room_id", rid)
```

- 第一个消息参数为人类可读短句，**简体中文**为主（与 ADR-0004 一致）。
- 后续为键值对；**键名**英文 `snake_case`。

## 级别指引

| 级别  | 用途 |
|-------|------|
| Debug | 开发期诊断，默认可关闭 |
| Info  | 关键业务里程碑、可审计路径 |
| Warn  | 可恢复问题、降级、重试 |
| Error | 需告警或影响用户体验/一致性 |

## 必带字段（在上下文可得时）

配置项 `logging.required_keys` 当前包含：`trace_id`、`user_id`、`room_id`。调用门面时应尽量在同一日志调用中携带，便于追踪与对账。

## 与 CI 的关系

- `verify-no-direct-logging`：拦截非法直调。
- `verify-log-calls`：校验门面 message 中文占比及 key 是否出现（实现见 `scripts/verify-log-calls.py`）。

## 与指标的边界

限流、幂等重放、抢答窗口、自动托管、重连结果、actor 队列深度与存储耗时以 Prometheus 指标为主；日志只记录异常、降级与需要审计的关键路径，避免把高频运行态写成日志噪声。
