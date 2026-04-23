---
title: 服务间通信与元数据
status: accepted
date: 2026-04-22
---

# ADR-0009 服务间通信与元数据

## 状态

已采纳。

## 背景

多进程后需在 `gate`、`lobby`、`room` 之间传递命令与可观测信息；须与 [ADR-0003](0003-frame-protocol.md) 的客户端帧协议解耦，且满足 `go-arch-lint` 的分层。

## 决策

1. **IDL**  
   - 集群间 RPC 使用 **gRPC + Protobuf**，放在 `api/proto/cluster/v1`，与 `api/proto/client/v1` **分目录** 维护。  
   - 在 Phase 2 内二树同属单一 Buf 模块 `api/proto`，**共用** `refs/tags/proto-baseline` 的 `buf breaking` 检查；不另起独立基线 tag，除非先完成 Buf 多模块治理。  

2. **链路元数据**  
   - 跨 gRPC 调用时透传 `trace_id`（及必要的 `user_id`/`room_id`）使用 **gRPC metadata**（`racoo-trace-id` 等名称在实现中统一）。  
   - 与 [ADR-0006](0006-logging-system-and-facade.md) 的日志必填字段一致，便于关联。  

3. **分层**  
   - `internal/mahjong` 与 `internal/domain` 不 import gRPC 或 `internal/cluster`（与 [docs/STORAGE.md](../../STORAGE.md) 同旨）。  
   - `internal/service` 侧保持编排与领域，**不直接依赖** `internal/cluster`；基础设施装配放在 `cmd/*` 与 `internal/app`（及必要的薄适配，若需可增独立包，见 go-arch 更新）。  
   - `internal/handler` 可调用面向集群的 **客户端接口**（在 `cmd` 注入），禁止 import `internal/store`（现规则）。  

4. **推送回客户端**  
   - `room` 产出领域事件，经 `cluster` 适配在 `gate` 上落地为 [ADR-0003](0003-frame-protocol.md) 帧；不在 `room` 进程内直接持 WebSocket。

## 后果

- 新 RPC 的破坏性变更会触发 `buf breaking`，与客户端 proto 同库时需一并慎重设计。  
- 若未来为 `cluster.v1` 独立 breaking 基线，须先拆分 Buf 模块并更新 [Makefile](../../Makefile) 的 `verify-proto-break`。
