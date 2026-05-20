---
description: topic 分支命名须符合 SSOT 正则，main 与受保护分支放行
alwaysApply: true
---

# Git 分支命名

- 短生命 topic 分支须匹配 `git.branch.topic_pattern`（与 Conventional Commits 的 type 前缀一致）。
- `git.default_branch`、`git.protected_branches` 与 `git.branch.allow_branches` 中的名称不校验 topic 模式。
- detached HEAD 下若 `git.branch.allow_detached_head` 为真则跳过（如 CI checkout）。

---

- **ADR**：`docs/adr/0007-git-workflow-policy.md`
- **Enforcer**：`scripts/verify-branch-name.py`
- **负例**：`.build/negatives/git_branch_bad_name.txt.neg`
