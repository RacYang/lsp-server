# scripts

仓库自动化与验证辅助脚本。

## 命名约定

- 可直接执行的 CLI 脚本使用 `kebab-case`，如 `verify-repo-hygiene.py`、`verify-git-push.sh`。
- 被 Python 脚本 import 的 helper 模块使用 `snake_case`，如 `lang_verify_common.py`，避免连字符破坏模块导入。
- `verify-*` 脚本必须保持只读；需要修改工作区的逻辑应放在 `generate`、`fix` 或专门的可变命令中。
