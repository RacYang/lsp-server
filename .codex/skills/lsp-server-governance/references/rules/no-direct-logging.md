---
description: 禁止业务代码直接 import slog/zap，须经统一日志门面
globs: ["internal/**/*.go", "cmd/**/*.go", "pkg/**/*.go"]
---

# 禁止直调底层日志

- 业务代码禁止直接 import `log/slog` 或 `go.uber.org/zap`。
- 所有日志调用必须通过 `pkg/logx` 门面，由门面统一处理字段注入、采样与 PII 脱敏。
- 门面实现文件（`logging.facade_impl_paths` 中声明的路径）是唯一豁免，可直接使用底层日志库。

---

- **ADR**：`docs/adr/0006-logging-system-and-facade.md`
- **Enforcer**：`scripts/verify-no-direct-logging.py`
- **负例**：`.build/negatives/lang_direct_slog_call.go.neg`
