---
title: 签名提交升级评估
status: accepted
date: 2026-04-28
---

# ADR-0029 签名提交升级评估

## 状态

已采纳。本 ADR 只采纳从 `recommended` 升级到 `required` 的评估路径，不立即修改 SSOT；CI 已接入不阻断的签名提交试运行。

## 背景

当前 `.build/config.yaml` 中 `git.signing.policy` 为 `recommended`，规则文件也说明未来若改为强制，
需要同步 ADR、enforcer 与负例。Phase 6 开始引入发布脚本与镜像 tag 校验后，提交来源可信度会影响发布追溯。

## 决策

### 1. 升级前置条件

- 团队成员完成 GPG 或 SSH signing key 分发。
- CI 能读取受信任公钥列表，且不会暴露私钥。
- 历史提交不要求补签；强制策略只约束新提交。

### 2. Enforcer 草案

若升级为 `required`，新增 `scripts/verify-signed-commit.py` 或等价 shell 脚本：

1. 读取待推送范围内的提交。
2. 使用 `git verify-commit` 或 GitHub verified metadata 校验签名。
3. 对 merge commit 与 bot commit 给出明确豁免规则。
4. 增加 `.build/negatives/` 负例，确保未签名提交被拒绝。

### 3. 暂不升级的原因

- 当前仓库仍处于 alpha 阶段，提交签名基础设施未验证。
- 强制签名会影响本地开发与 CI bot 账号，需要先完成试运行。
- 发布 tag 已由 `deploy/release` 脚本强制校验签名，可先覆盖生产发布入口。

## 后果

- 本 ADR 不修改 `.build/config.yaml`。
- `.github/workflows/ci.yml` 已通过 `scripts/verify-signed-commit-trial.sh` 接入不阻断试运行，未签名提交或未受信公钥只输出 warning。
- 待签名试运行完成后，再以独立 PR 把 `git.signing.policy` 改为 `required`。
- 若升级，需同步修订 ADR-0007 与 `.cursor/rules/git-signed-commits.mdc`。
