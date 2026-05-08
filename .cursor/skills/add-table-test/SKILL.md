---
name: add-table-test
description: 新增表驱动单元测试。用于为 Go 包补充可读、可扩展、可回归的输入输出测试。
---

# 新增表驱动测试

## When to use

当修复 bug、扩展规则、增加边界条件或补齐共享逻辑覆盖率时使用。

## Inputs

- 被测包与函数。
- 用例名称、输入、期望输出与错误语义。
- 是否需要复用 `.cursor/templates/code/go-table-test/manifest.yaml`。

## Steps

1. 优先在被测包旁新增或扩展 `_test.go`。
2. 使用 `tests := []struct{ ... }` 描述用例，名称使用中文或简明业务语义。
3. 断言完整对象优于逐字段散点断言。
4. 对 bug 修复先写失败用例，再修改实现。

## Verify

- 运行目标包 `go test ./path/to/pkg`。
- 涉及共享包时运行 `make verify-test-fast`。
