# logx

统一日志门面包（`racoo.cn/lsp/pkg/logx`）。Phase 1 已落地基于标准库 `log/slog` 的 JSON 后端、Context 自动注入、子 logger、采样框架、键名层脱敏与测试 Observer；业务代码仅通过本包写日志，禁止在豁免路径之外直接 `import log/slog`。

## 用法

```go
ctx := context.Background()
ctx = logx.WithTraceID(ctx, tid)
ctx = logx.WithUserID(ctx, uid)
ctx = logx.WithRoomID(ctx, rid)
log := logx.New(os.Stdout, logx.LevelInfo).With("op", "join_room")
log.Info(ctx, "玩家进入房间", "rule_id", "sichuan_xzdd")
```

包级函数 `logx.Info` 等使用默认 Logger（标准输出、Info 级别）。

测试优先使用 Observer：

```go
obs, log := logx.NewObserver()
log.Info(ctx, "玩家进入房间", "rule_id", "sichuan_xzdd")
entries := obs.Drain()
```

## 治理

详见 [docs/LOGGING.md](../../docs/LOGGING.md) 与 [ADR-0006](../../docs/adr/0006-logging-system-and-facade.md)。`make verify-lang` 中的日志调用与边界校验对业务包生效；`pkg/logx/**` 为门面实现豁免区。
