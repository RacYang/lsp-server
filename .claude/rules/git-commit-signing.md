---
description: 提交签名当前为推荐策略，逐步向强制演进
alwaysApply: true
---

# Git 提交签名

- 当前 `git.signing.policy` 为 `recommended`：鼓励在本地与 CI 中启用 SSH/GPG 提交签名。
- CI 中通过 `verify-signed-commit-trial.sh` 试用签名校验，失败时仅警告不阻断。
- 若未来改为 `required`，须同步更新 ADR、SSOT 与 enforcer。

---

- **ADR**：`docs/adr/0007-git-workflow-policy.md`
- **Enforcer**：`scripts/verify-signed-commit-trial.sh`
- **负例**：`.build/negatives/commit_non_unified_summary.txt.neg`
