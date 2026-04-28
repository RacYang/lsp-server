---
title: 语言与书写策略
status: accepted
date: 2026-04-22
---

# ADR-0004 语言与书写策略

## 状态

已采纳。

## 决策

仓库内面向人类阅读的说明性文字以**简体中文**为主，包括：设计文档、ADR、规则（`.mdc`）正文、技能说明（`SKILL.md`）、`README`、Go 源码注释、以及通过统一日志门面输出的**日志 message 文案**。

## 适用范围

- Markdown / MDC / 技能文档的正文（frontmatter 的键名可保持英文以便工具解析）。
- Go 代码中的 `//` 与 `/* */` 注释（不含字符串字面量、不含生成代码头约定块）。
- 业务代码经 `pkg/logx` 发出的可见日志 message（首条人类可读短句）。

## 允许的英文与例外

以下情形**允许**使用英文或保持机器友好格式，不视为违反本 ADR：

- Go 标识符、包名、导入路径、文件名、命令行与配置键名。
- 技术关键词（示例：`Goroutine`、`Channel`、`Context`、`Redis`、`PostgreSQL`、`gRPC`、`Protobuf`、`WebSocket`、`etcd`、`Prometheus`、`pprof`），以及业界通用缩写（`HTTP`、`TCP`、`JSON` 等）。
- `errors.New` / `fmt.Errorf` 返回给调用方的 **error 字符串**：保持英文小写、无尾标点，便于错误链与监控聚合（与 [docs/CODING-STYLE.md](../CODING-STYLE.md) 一致）。
- 结构化日志的 **字段名（key）**：使用英文 `snake_case`（见 [ADR-0006](0006-logging-system-and-facade.md)）。
- 代码块内的示例代码、命令输出、路径、版本号、URL。

## 硬约束与 SSOT

语言类硬检查由 `scripts/verify-lang-docs.py`、`scripts/verify-lang-comments.py`、`scripts/verify-log-calls.py` 执行；阈值、扫描路径与排除项以 [.build/config.yaml](../../.build/config.yaml) 的 `language`、`commenting`、`logging` 节为**唯一配置源**。

## 注释体系与日志体系

- 注释「写什么、写在哪一层」由 [ADR-0005](0005-comment-system.md) 约束（以规范为主，硬编码风格检查为辅）。
- 日志门面、禁止直调底层 logger、字段与级别约定由 [ADR-0006](0006-logging-system-and-facade.md) 约束。

## 后果

- 文档与协作默认语言为中文，降低团队阅读成本。
- 错误字符串与日志 key 保持英文惯例，兼容生态工具与查询习惯。
- 新增 enforcer 与负例样本纳入 `make verify`，避免策略漂移。
