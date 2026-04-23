---
title: Git 工作流策略
status: accepted
date: 2026-04-22
---

# ADR-0007 Git 工作流策略

## 背景

仓库已具备提交信息、语言与治理流水线等约束，但分支、推送、标签与 hook 与 CI 的一致性尚未纳入单一事实源。需要把 Git 操作纳入与 `.build/config.yaml`、ADR、规则、enforcer、负例相同的治理闭环。

## 决策

1. **分支模型**：采用 **GitHub Flow**。`main` 为唯一长生命主干；短生命 **topic 分支** 以 `type/描述` 命名，`type` 与 Conventional Commits 类型对齐（如 `feat/`、`fix/`、`docs/`）。
2. **合并策略**：向 `main` 的合并在托管侧以 **squash merge** 为主；本地 hook 无法替代托管侧分支保护，本 ADR 约束「语义与校验分层」。
3. **受保护分支**：`main` 禁止 **非 fast-forward** 更新（含改写历史式强推）；更细粒度的「仅 PR 可合入」由托管侧分支保护承担。
4. **标签**：发布标签使用 `vX.Y.Z`（可选预发布后缀）；`proto-baseline` 等工具基线标签列入白名单。
5. **提交 Trailer**：`Made-with` 等由工具自动注入的 trailer 暂不纳入 Phase 1 硬禁止；Phase 2 再收紧 trailer 允许集。
6. **校验分层**：
   - **repo 上下文**：`make verify-git-repo`（纳入 `verify` / `verify-fast`）。
   - **本地分支上下文**：`make verify-git-local`（仅 `verify-pre-commit` 链）。
   - **推送上下文**：`make verify-git-push`（仅 `pre-push`）。
7. **Hook 与 CI 映射**：以 SSOT `git.ci_parity` 为准：`pre-commit` → `verify-pre-commit`，`pre-push` → `verify`，CI → `verify`。

## 后果

- 开发者须使用 topic 分支命名；在 `main` 上直接开发不受命名规则误伤（显式放行）。
- 推送前校验与 CI 共享同一套 `make verify` 目标集合，减少「本地过、CI 挂」的漂移。
- 后续若引入 `develop` 或多保护分支，仅需扩展 SSOT 与放行列表，并评估是否修订本 ADR。

## 相关

- [ADR-0000](./0000-engineering-charter.md) 工程宪章
- [ADR-0004](./0004-language-and-writing-policy.md) 语言与书写策略
- `.cursor/rules/git-*.mdc` 可执行规则
