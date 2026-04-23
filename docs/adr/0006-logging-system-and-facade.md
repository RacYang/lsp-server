---
title: 日志体系与统一门面
status: accepted
date: 2026-04-22
---

# ADR-0006 日志体系与统一门面

## 决策

业务与领域代码**禁止**直接依赖或调用标准库 `log/slog` 与 `go.uber.org/zap`（含 `zapcore`）；统一通过模块 `racoo.cn/lsp/pkg/logx`（门面）输出日志。门面**实现**可在 `pkg/logx/**` 内直调底层实现，该路径由 SSOT 豁免直调检查。

## Phase 说明

- **Phase 0（当前）**：仅定义门面约定、文档与 enforcer；**不强制**在仓库中提交完整 `logx` 实现（避免过早业务代码）。业务目录无 Go 文件时，日志相关检查自然跳过。
- **Phase 1+**：首次引入业务日志时，在 `pkg/logx` 落地实现，并保持与本文档一致。

## 级别与字段

- **级别**：`Debug` 仅用于开发/可关闭诊断；`Info` 记录关键业务里程碑；`Warn` 可恢复异常或降级；`Error` 需关注且可能影响用户体验或数据一致性。
- **结构化 key**：英文 `snake_case`；同一语义全局同名。
- **建议携带字段**（在上下文可得时）：`trace_id`、`user_id`、`room_id`；与玩法相关可扩展 `rule_id`、`op` 等。

## 日志 message 语言

- 面向人读的 message 主体使用**简体中文**（与 ADR-0004 一致）；技术令牌、ID 片段可保留原样。

## 硬约束

- `scripts/verify-no-direct-logging.py`：在非豁免路径发现禁止的 import 即失败。
- `scripts/verify-log-calls.py`：对门面调用的首条 message 做中文占比校验，并检查是否包含配置要求的结构化 key 名（见 SSOT `logging.required_keys`）。

## 后果

日志格式与依赖收口到单点，便于替换实现、接入采集与统一治理；业务代码不散落底层 API。
