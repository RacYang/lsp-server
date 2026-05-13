---
description: 文档与规则正文以中文为主（剥离代码块后统计）
globs: ["**/*.md"]
---

# 文档中文约束

- 仓库内 Markdown 文档正文（剥离代码块后）必须以简体中文为主。
- 中文占比阈值与受检路径由 `.build/config.yaml` 的 `language` 节定义，不得绕过。
- 英文仅允许出现在代码块、frontmatter、URL、文件路径与 SSOT 关键字白名单中。

---

- **ADR**：`docs/adr/0004-language-and-writing-policy.md`
- **Enforcer**：`scripts/verify-lang-docs.py`
- **负例**：`.build/negatives/lang_docs_full_english.md.neg`
