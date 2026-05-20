---
description: 向受保护分支的合并须以 squash 方式，禁止本地 merge commit 推入
alwaysApply: true
---

# Git 合并策略

- 向 `main` 的合并在托管侧以 squash merge 完成；本地不得产生面向受保护分支的 merge commit。
- 向受保护分支推送包含 merge commit 的历史会被 pre-push hook 拒绝（non-fast-forward 检测）。

---

- **ADR**：`docs/adr/0007-git-workflow-policy.md`
- **Enforcer**：`scripts/verify-protected-branch-push.sh`
- **负例**：`.build/negatives/git_protected_branch_push.txt.neg`
