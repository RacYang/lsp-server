---
description: 工作树禁止出现 macOS Finder 副本类目录或文件
alwaysApply: true
---

# Git 工作树卫生

- 当 `git.repo_hygiene.workspace_space_dirs_blocked` 为真时，工作树（含未跟踪文件）不得出现以「空格 + 数字」结尾命名的目录或文件（如 `redis 2`、`config 3.yaml`）。
- 这是 macOS Finder 在重名复制时的默认行为，副本通常不会被跟踪却长期污染索引。
- 如确需保留某路径，可加入 `git.repo_hygiene.workspace_scan_excludes`（fnmatch 风格 glob）。

---

- **ADR**：`docs/adr/0007-git-workflow-policy.md`
- **Enforcer**：`scripts/verify-repo-hygiene.py`
- **负例**：`.build/negatives/git_repo_hygiene_space_dir.txt.neg`
