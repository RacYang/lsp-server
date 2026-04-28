---
name: add-adr
description: 新增架构决策记录。用于引入新的治理、架构、协议、运维或规则集决策，并同步 ADR 索引与阶段文档。
---

# 新增 ADR

## When to use

当变更涉及架构边界、稳定契约、治理硬约束、部署运维策略、规则集引入或跨模块长期约定时使用。

## Inputs

- ADR 编号与主题：编号递增，文件名使用 `NNNN-topic.md`。
- 决策状态：通常从 `draft` 开始，采纳后改为 `accepted`。
- 影响面：README、CHANGELOG、AGENTS.md 或相关专题文档。

## Steps

1. 在 `docs/adr/` 新增 ADR 文件，frontmatter 必须包含 `title`、`status`、`date`。
2. 正文至少包含 `## 状态`、`## 背景`、`## 决策`、`## 后果`。
3. 如 ADR 引入硬约束，同时补 `.cursor/rules/*.mdc`、enforcer 与 `.build/negatives/*.neg`。
4. 更新 `docs/adr/README.md` 的分阶段索引。
5. 按影响面同步 `README.md`、`CHANGELOG.md` 与专题文档。

## Verify

- 运行 `make verify-meta`。
- 文档范围较大时运行 `make verify-fast`。
