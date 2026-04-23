# 贡献指南

1. 首次请运行 `make bootstrap`。
2. 变更 proto 或治理配置后运行 `make generate`。
3. 推送前运行 `make verify`。
4. 提交信息使用 `type(scope): 摘要` 或 `type: 摘要`。
5. `type` 仅允许仓库生成的 conventional commit 类型；`scope` 可省略，填写时使用小写英文、数字、短横线或 `/`。
6. `摘要` 以简体中文短语为主，不要写英文整句，也不要以句号、感叹号等收尾。

## 分支与合并（GitHub Flow）

- 长生命分支仅为 `main`；日常开发使用短生命 **topic 分支**，命名形如 `feat/room-state`、`fix/login`，前缀与 Conventional Commits 的 `type` 一致。
- 向 `main` 合入在托管侧以 **squash merge** 为主；强推非受保护分支时优先使用 `git push --force-with-lease`。
- 已安装 hook 时，`pre-commit` 会跑 `make verify-pre-commit`，`pre-push` 会先跑 `make verify-git-push` 再跑 `make verify`；未安装 hook 时请在推送前手跑上述命令。
- 仓库级与 hook 映射校验：`make verify-git-repo`（已包含在 `make verify` / `make verify-fast` 中）。
