---
description: 新增架构决策记录。用于引入新的治理、架构、协议、运维或规则集决策，并同步 ADR 索引与阶段文档。
---

# 新增 ADR

## When to use

当变更涉及架构边界、稳定契约、治理硬约束、部署运维策略、规则集引入或跨模块长期约定时使用。

## Inputs

- ADR 编号与主题：编号递增，文件名使用 `NNNN-topic.md`。
- 决策状态：通常从 `draft` 开始，采纳后改为 `accepted`。
- 影响面：README、CHANGELOG、CLAUDE.md 或相关专题文档。

## Steps

1. **确认无重叠覆盖**：检索现有 rules/commands/ADR，确认无已有资产覆盖此场景；列出最近相关资产差异（ADR-0049）。
2. 在 `docs/adr/` 新增 ADR 文件，frontmatter 必须包含 `title`、`date`。
3. 正文必须包含 `## 决策`（bullet 列表）与 `## 后果`（bullet 列表）；`## 背景` 仅在无法自解释时加，最多 5 行；不加 `## 状态` 节。
4. 如 ADR 引入硬约束，本命令只记录决策；随后必须切换到 `/add-constraint` 完成 rule、enforcer 与负例闭环。
5. 更新 `docs/adr/README.md` 的分阶段索引。
6. 按影响面同步 `README.md`、`CHANGELOG.md` 与专题文档。

## Verify

- 运行 `python3 scripts/verify-adr-numbering.py`（ADR 编号连续、README 已更新）。
- 运行 `python3 scripts/verify-doc-links.py`（新 ADR 引用路径不悬空）。
- 运行 `make verify-skeleton`。
- 运行 `make verify`。
