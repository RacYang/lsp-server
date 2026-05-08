---
name: add-integration-test
description: 新增集成测试。用于覆盖跨 app、handler、room service 或无 Docker 重连链路的行为。
---

# 新增集成测试

## When to use

当行为跨越多个包、涉及重连、幂等、托管超时、限流或持久化回放时使用。

## Inputs

- 目标链路与成功条件。
- 是否需要 `integration` build tag。
- 外部依赖是否可用；无 Docker 链路优先。

## Steps

1. 在现有 integration 测试包中寻找相近夹具。
2. 复用 fake clock、内存 hub 或无 Docker 存储替身。
3. 给测试名描述用户可见行为，而不是实现细节。
4. 如需 CI 覆盖，确认 Makefile 的 integration 目标包含该测试。

## Verify

- 运行目标 `go test -tags=integration ... -run TestName -count=1`。
- 对无 Docker 链路运行 `RUN_INTEGRATION=1 make verify-test-integration-nodocker`。
