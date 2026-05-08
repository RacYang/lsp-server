---
name: add-deploy-overlay
description: 新增部署 overlay 或运行手册。用于扩展部署参数、单机/集群差异与运维说明。
---

# 新增部署覆盖

## When to use

当新增部署形态、环境覆盖、Secret 引用方式或运维 runbook 时使用。

## Inputs

- 部署目标环境。
- 配置差异与敏感信息边界。
- 关联 ADR 或运维文档。

## Steps

1. 不提交密钥、凭据或真实 `.env`。
2. 把长期决策写入 ADR，把操作步骤写入 runbook。
3. 更新示例配置时保持默认安全。
4. 确认文档以中文为主。

## Verify

- 运行 `make verify-secrets`。
- 运行 `make verify-meta`。
