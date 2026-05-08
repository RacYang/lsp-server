---
name: add-template
description: 新增模板资产。用于增加代码骨架、注释样板、文件头或文档骨架。
---

# 新增模板

## When to use

当某类文件反复需要相同骨架，且仅靠规则文字无法保证一致时使用。

## Inputs

- template ID。
- 适用路径 glob。
- 关联 skill、rule 与覆盖矩阵单元。

## Steps

1. 在 `.cursor/templates/{code,comment,header,doc}/` 下创建目录。
2. 编写 `manifest.yaml`，只使用 presence、regex、script 三种 verify hook。
3. 添加模板文件并更新 `.cursor/templates/README.md`。
4. 更新 coverage matrix 与相关 skill。

## Verify

- 运行 `make verify-skeleton`。
- 运行 `make verify-doc-links`。
