---
title: Agent 治理体系职责边界
status: accepted
date: 2026-05-08
---

# ADR-0042 Agent 治理体系职责边界

## 状态

已采纳。

## 背景

本仓库已经具备 `AGENTS.md`、`.cursor/rules/`、`.cursor/skills/`、`.build/config.yaml`、负例与 `make verify`，但这些资产的边界曾经混用：入口文件复述硬约束，规则目录同时承载可执行约束与原则说明，技能文档重复 ADR 与 rule 的内容，代码骨架和注释样板没有统一落点。

这种混用会让 agent 在执行任务时靠猜测选择入口，容易出现范围扩散、跳过验证、局部补丁堆叠和风格漂移。需要把面向 agent 的治理资产定义为完整体系，并让 harness 持续校验体系自身不腐化。

## 决策

1. Agent 治理体系由 **5 类内容资产 + 1 套 harness** 组成：`AGENTS.md`、ADR、rules、skills、templates 与 harness。
2. `AGENTS.md` 是入口路由，只保留不可妥协纪律、任务路由、导航与边界声明；不得复述已经由 enforcer 覆盖的硬约束细节。
3. ADR 记录长期决策与原则。没有自动校验的 norm 类说明必须落到 ADR 或专题文档，不再作为 `.cursor/rules/*.mdc` 独立存在。
4. `.cursor/rules/*.mdc` 只承载硬约束。每条 rule 必须是 `kind: constraint`，并声明 `adr`、`enforcer` 与 `negative_test` 三元组。
5. `.cursor/skills/*/SKILL.md` 只承载任务流程，固定为 `When to use`、`Inputs`、`Steps`、`Verify` 四类内容，不复述 rule 的执行细节。
6. `.cursor/templates/` 承载代码骨架、注释样板、文件头与文档骨架。每个模板目录必须包含 `manifest.yaml`，声明用途、适用范围、覆盖矩阵单元、关联规则与校验钩子。
7. `.build/config.yaml` 新增 `governance` 节，作为 AGENTS 行数、rules/skills/templates 数量上限与必需锚点的 SSOT。
8. harness 必须校验前五类资产的形态、链接与最小语义：包括 schema 校验、文档链接校验、骨架校验、负例闭环与 AGENTS 形态校验。
9. 覆盖矩阵作为活文档维护在 `docs/agent-governance/coverage-matrix.md`，记录生命周期、架构层与横切关注的覆盖情况；它不是 ADR，允许随任务体系演进。
10. 体系自身的演进必须通过 meta-skills 完成，例如新增 skill、template 或退役旧资产；禁止散点修改。

## 资产边界

| 角色 | 物理位置 | 职责 | 禁忌 |
|------|----------|------|------|
| manual | `AGENTS.md` | 入口路由与不可妥协纪律 | 不复述硬约束细节 |
| decisions | `docs/adr/` | 记录长期决策、原则与后果 | 不写逐步操作手册 |
| constraints | `.cursor/rules/` | 可执行硬约束 | 不承载 norm 类说明 |
| workflows | `.cursor/skills/` | 任务流程 | 不重复 enforcer 细节 |
| scaffolds | `.cursor/templates/` | 代码、注释、文件头与文档骨架 | 不脱离 manifest 与校验 |
| harness | `.build/`、`scripts/`、`Makefile`、`.githooks/` | 自动验证治理资产 | 不依赖人工记忆 |

## 后果

- `.cursor/rules/` 的语义收敛为“打开就是会被 CI 打红的事”。
- `AGENTS.md` 变薄，陌生 agent 可从任务路由直接找到对应 skill、template、rule 与 ADR。
- 代码骨架、注释样板和文件头从口头约定变为可复用资产，并逐步纳入 `verify-skeleton`。
- 新增规则、技能、模板或退役旧资产都需要同步 coverage matrix 与 harness，避免治理体系再次打补丁式增长。

## 相关

- [ADR-0000](0000-engineering-charter.md) 工程宪章
- [ADR-0005](0005-comment-system.md) 注释体系
- [ADR-0006](0006-logging-system-and-facade.md) 日志体系与统一门面
- [ADR-0007](0007-git-workflow-policy.md) Git 工作流策略
