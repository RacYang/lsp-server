# 中文化策略

本文档汇总 ADR-0004 的书写策略，并把文档、注释、日志三类语言约束集中到同一处。具体阈值仍以 `.build/config.yaml` 为 SSOT。

## 适用范围

- 文档与规则正文：`language.docs_paths` 覆盖的 Markdown、MDC 与 SKILL 文档，剥离代码块、行内代码和 URL 后统计。
- Go 注释：`commenting.code_paths` 覆盖的源码注释文本，生成代码与豁免路径由 `commenting.code_exclude` 排除。
- 日志 message：经 `pkg/logx` 门面输出的人类可读消息，结构化字段仍使用英文 `snake_case`。

## 当前阈值

| 类型 | SSOT 字段 | 当前含义 |
|------|-----------|----------|
| 文档与规则正文 | `language.docs_min_cjk_ratio` | 中文占比不得低于配置阈值 |
| Go 注释 | `commenting.min_cjk_ratio` | 注释文本中文占比不得低于配置阈值 |
| 日志 message | `logging.message_min_cjk_ratio` | 门面日志 message 中文占比不得低于配置阈值 |

技术关键词（如 Redis、PostgreSQL、gRPC、Protobuf、WebSocket、Prometheus、pprof）可保持英文，统计脚本会按 SSOT 中的关键词白名单处理。

## 写作口径

- 文档解释决策、约束与运维口径，不把英文 README 直译成松散条目。
- 注释优先解释「为什么」与「边界」，避免重复代码字面含义。
- 日志 message 面向排障与审计，使用简体中文短句；字段名面向机器聚合，使用英文 `snake_case`。

## 校验入口

- `scripts/verify-lang-docs.py`：文档与规则正文中文占比。
- `scripts/verify-lang-comments.py`：Go 注释中文占比。
- `scripts/verify-log-calls.py`：日志 message 中文占比与必带字段。
- `make verify-lang`：聚合上述检查，并包含禁止直连底层 logger 的校验。
