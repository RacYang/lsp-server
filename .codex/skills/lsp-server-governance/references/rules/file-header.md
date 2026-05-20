---
description: 脚本文件必须使用标准文件头
globs: ["scripts/*"]
---

# 文件头

- Shell 脚本使用 `#!/usr/bin/env bash` 与 `set -euo pipefail`。
- Python 校验脚本使用 `#!/usr/bin/env python3`。

---

- **ADR**：`docs/adr/0042-agent-governance.md`
- **Enforcer**：`scripts/verify-source-shape.py`
- **负例**：`.build/negatives/file_header_missing.sh.neg`
