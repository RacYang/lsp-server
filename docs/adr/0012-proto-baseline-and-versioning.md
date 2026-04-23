---
title: Proto 基线演进与版本策略
status: accepted
date: 2026-04-22
---

# ADR-0012 Proto 基线演进与版本策略

## 状态

已采纳。

## 背景

仓库使用 [ADR-0000](0000-engineering-charter.md) 的 `proto.baseline_ref` 与 `buf breaking` 防止无意破坏。Phase 2 同时增加 `api/proto/cluster/v1` 与扩展 `api/proto/client/v1`，需避免「改 tag 掩盖 breaking」或「多基线与 schema 不一致」。

## 决策

1. **单模块阶段**（Phase 2 默认）  
   - 根 [buf.yaml](../../buf.yaml) 仅含 `api/proto` 一个模块；`client.v1` 与 `cluster.v1` 均在同一 `buf breaking --against` 下检测。  
   - **不** 为 `cluster.v1` 单独打 `refs/tags/*-proto-baseline`，直至 Buf 多模块与 [.build/schema/config.schema.json](../../.build/schema/config.schema.json) 可承载多 baseline。  

2. **客户端 v1 兼容**  
   - 以 **仅追加** `message` 字段、**仅追加** `oneof` 变体、**仅追加** 枚举值为主；`buf breaking` 须保持通过。  
   - 若必须不兼容，引入 **`client.v2` 新包/新目录**，旧 `client.v1` 基线不动，客户端分版本升级。  

3. **禁止行为**  
   - 为通过 CI 而移动、篡改已发布的 `proto-baseline` tag 内容（除维护者显式大版本管理外）。  
   - 在 cluster proto 中复用会污染客户端命名空间的 `package` 名称。  

4. **生成物**  
   - `buf generate` 后 `api/gen/go` 的 diff 必须在 `make verify` 中为零（与现 Makefile 行为一致）。  

## 后果

- `cluster.v1` 早期迭代仍受 `buf breaking` 保护，与客户端变更同一 MR 时评审需同时看两条契约树。  
- 日后再拆多模块时，可单独发 ADR 与迁移脚本。  

## 与「硬约束负例」

- 若后续增加「禁止在 client.v1 中删除已编号字段」的 enforcer，可附负例 proto（`.build/negatives`），非本 ADR 强制的先行条件。  
