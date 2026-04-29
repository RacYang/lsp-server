---
title: 日志体系与统一门面
status: accepted
date: 2026-04-22
---

# ADR-0006 日志体系与统一门面

## 状态

已采纳。

## 修订

- 2026-04-29：v2，将日志治理从调用现场三字段字面量升级为 Context 自动注入，并引入字段命名、日志/指标边界与 PII 总则；采样、动态级别与 OTel 细节分别见 ADR-0033、ADR-0034、ADR-0035。

## 背景

原始版本只约束业务代码不得直调底层 logger，并要求每条日志调用显式携带 `trace_id`、`user_id`、`room_id`。该做法能快速建立治理基线，但会带来重复书写、空值占位和 OTel trace 字段双源问题。

## 决策

业务与领域代码禁止直接依赖或调用标准库 `log/slog` 与 `go.uber.org/zap`（含 `zapcore`）；统一通过模块 `racoo.cn/lsp/pkg/logx`（门面）输出日志。门面实现可在 `pkg/logx/**` 内直调底层实现，该路径由 SSOT 豁免直调检查。

`trace_id`、`user_id` 与 `room_id` 在业务边界写入 `context.Context`，由 `logx` Handler 自动合并进输出。业务调用现场不得手写这些上下文字段，避免与 OTel 注入或统一边界注入产生双源覆盖。

## Phase 说明

- **Phase 0**：定义门面约定、文档与 enforcer。
- **Phase 1+**：`pkg/logx` 落地 Context 注入、字段治理、测试 Observer 与基础运行时配置。
- **Phase 6+**：采样阈值、动态级别 HTTP 管控与 OTel logs bridge 按独立 ADR 演进。

## 级别与字段

- **级别**：`Debug` 仅用于开发/可关闭诊断；`Info` 记录关键业务里程碑；`Warn` 可恢复异常或降级；`Error` 需关注且可能影响用户体验或数据一致性。
- **结构化 key**：英文 `snake_case`，长度受 SSOT `logging.field_naming` 限制。
- **核心字段**：`trace_id`、`span_id`、`user_id`、`room_id`、`rule_id`、`op`、`err`、`elapsed_ms`、`shard`、`region` 等跨服务字段全局同名。
- **业务字段**：无需全部进入 SSOT，但必须满足命名规则，不得使用敏感键或指标候选键。

## 日志 message 语言

- 面向人读的 message 主体使用**简体中文**（与 ADR-0004 一致）；技术令牌、ID 片段可保留原样。

## 硬约束

- `scripts/verify-no-direct-logging.py`：在非豁免路径发现禁止的 import 即失败。
- `scripts/verify-log-calls.py`：校验门面调用的 message 中文占比、字段命名、敏感键、指标候选键，并禁止业务调用现场手写上下文字段。
- `scripts/verify-log-boundaries.py`：对已存在的日志边界文件校验 `logx.WithTraceID`、`logx.WithUserID`、`logx.WithRoomID` 注入。

## 与指标边界

高频运行态数据进入 Prometheus 指标；日志只记录异常、降级、审计与关键业务里程碑。`logging.forbidden_field_keys` 中的键不得作为日志字段。

## 后果

日志格式、字段、PII 基线与依赖收口到单点，便于替换实现、接入采集与统一治理；调用现场更简洁，但边界文件必须负责写入 Context 字段。
