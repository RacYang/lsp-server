---
description: 新增 Prometheus 指标。用于扩展运行态观测、容量基线或 SLO 观察项。
---

# 新增指标

## When to use

当需要观测请求量、延迟、容量、队列深度、重连或错误率等运行态数据时使用。

## Inputs

- 指标名、类型、标签与单位。
- 指标所属包与采集边界。
- 对应 ADR 或 SLO 背景。

## Steps

1. 指标名使用 `lsp_` 前缀和允许后缀。
2. 高基数字段不得进入标签。
3. 日志只记录异常与里程碑，高频运行态进入指标。
4. 更新相关文档或 runbook。

## Verify

- 运行 `make verify-metrics-naming`。
- 涉及日志边界时运行 `make verify-lang`。
