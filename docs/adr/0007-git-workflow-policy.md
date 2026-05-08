---
title: Git 工作流策略
status: accepted
date: 2026-04-22
---

# ADR-0007 Git 工作流策略

## 状态

已采纳。

## 背景

仓库已具备提交信息、语言与治理流水线等约束，但分支、推送、标签与 hook 与 CI 的一致性尚未纳入单一事实源。需要把 Git 操作纳入与 `.build/config.yaml`、ADR、规则、enforcer、负例相同的治理闭环。

## 决策

1. **分支模型**：采用 **GitHub Flow**。`main` 为唯一长生命主干；短生命 **topic 分支** 以 `type/描述` 命名，`type` 与 Conventional Commits 类型对齐（如 `feat/`、`fix/`、`docs/`）。
2. **合并策略**：向 `main` 的合并在托管侧以 **squash merge** 为主；本地 hook 无法替代托管侧分支保护，本 ADR 约束「语义与校验分层」。
3. **受保护分支**：`main` 禁止 **非 fast-forward** 更新（含改写历史式强推）；更细粒度的「仅 PR 可合入」由托管侧分支保护承担。
4. **标签**：发布标签使用 `vX.Y.Z`（可选预发布后缀）；`proto-baseline` 等工具基线标签列入白名单。
5. **提交 Trailer**：提交历史禁止出现 SSOT 明确禁用的 trailer；当前 `Made-with` 属于硬禁止项，本地 `commit-msg` hook 需在校验前剥离该类工具注入 trailer。
6. **校验分层**：
   - **repo 上下文**：`make verify-git-repo`（纳入 `verify` / `verify-fast`）。
   - **本地分支上下文**：`make verify-git-local`（仅 `verify-pre-commit` 链）。
   - **推送上下文**：`make verify-git-push`（仅 `pre-push`）。
7. **Hook 与 CI 映射**：以 SSOT `git.ci_parity` 为准：`pre-commit` → `verify-pre-commit`，`pre-push` → `verify`，CI → `verify`。

## 后果

- 开发者须使用 topic 分支命名；在 `main` 上直接开发不受命名规则误伤（显式放行）。
- 推送前校验与 CI 共享同一套 `make verify` 目标集合，减少「本地过、CI 挂」的漂移。
- 后续若引入 `develop` 或多保护分支，仅需扩展 SSOT 与放行列表，并评估是否修订本 ADR。

## 操作建议

- 向 `main` 的合并在托管侧以 squash merge 为主；禁止普通 merge commit 直接落到 `main` 主要由托管侧「仅允许 squash」按钮与分支保护实现，本地 hook 无法完整替代。
- 向非受保护分支强推时，优先使用 `git push --force-with-lease`，降低覆盖他人新提交的风险；`pre-push` 无法区分 `--force` 与 `--force-with-lease`，因此本项为流程规范，不作为本地硬校验。
- `git.signing.policy` 当前为 `recommended`：鼓励在本地与 CI 逐步启用签名提交；若未来改为强制，应同步修订 ADR、SSOT、enforcer 与负例。

## 相关

- [ADR-0000](0000-engineering-charter.md) 工程宪章
- [ADR-0004](0004-language-and-writing-policy.md) 语言与书写策略
- [ADR-0029](0029-signed-commit-required.md) 签名提交升级评估
- `.cursor/rules/git-*.mdc` 可执行规则
