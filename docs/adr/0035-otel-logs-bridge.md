---
title: OTel Logs Bridge 评估
status: accepted
date: 2026-04-29
---

# ADR-0035 OTel Logs Bridge 评估

## 状态

已采纳。

## 背景

成熟可观测体系通常把日志、指标与 trace 关联起来。Go 生态中 OTel 可从 `context.Context` 读取 span 上下文，并把 `trace_id`、`span_id` 与日志关联。

## 决策

`go.opentelemetry.io/contrib/bridges/otelslog` 与 OTel SDK 不在当前 `deps.denylist` 中。本仓允许为日志桥接预留 `otel` build tag，但默认构建不启用，`logging.otel.enabled` 默认保持 `false`。

Phase 1 只在 `pkg/logx` 的 `otel` build tag 文件中从 OTel span context 注入 `trace_id` 与 `span_id`。完整 OTLP logs exporter、采集端配置与运行时开关接线留待生产观测链路确定后推进。

## 后果

日志门面可以与 OTel 方向兼容，但默认路径不新增运行时依赖，也不改变本地开发和 CI 的构建行为。
