---
description: 禁止向受保护分支强推，推荐使用 force-with-lease
alwaysApply: true
---

# Git 强推策略

- 禁止向 `git.protected_branches` 中分支执行 force push（含 `--force`、`-f`）。
- 向非受保护分支强推时优先使用 `git push --force-with-lease`，避免覆盖他人提交。
- 禁止直接向受保护分支提交（`reject_direct_push`），所有变更须通过 PR 合入。

---

- **ADR**：`docs/adr/0007-git-workflow-policy.md`
- **Enforcer**：`scripts/verify-protected-branch-push.sh`
- **负例**：`.build/negatives/git_protected_branch_push.txt.neg`
