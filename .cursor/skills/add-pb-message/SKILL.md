---
name: add-pb-message
description: 为客户端或 RPC 契约新增 Protobuf 消息。用于编辑 `api/proto`、引入新消息 ID 或扩展房间与大厅 API。
---

# 新增 PB 消息

## When to use

当需要新增客户端消息、扩展 `cluster.v1` RPC 契约、引入新的消息 ID，或调整房间/大厅 API 字段时使用。

## Inputs

- 契约归属：`api/proto/client/v1` 或 `api/proto/cluster/v1`。
- 兼容性判断：字段编号、oneof 变体、枚举值是否仅追加。
- 调用方：handler、room/lobby/gate gRPC 服务与测试。

## Steps

1. 将消息放在 `api/proto/client/v1` 或 `api/proto/cluster/v1`，保持 `package` 与 `go_package` 稳定。
2. 在既有 message、oneof、enum 中只追加编号；删除或复用编号必须先走新版本包或独立 ADR。
3. 运行 `make generate` 生成 `api/gen/go`，确认生成物与 proto 一起提交。
4. 契约存在后再更新 handler、帧路由、gRPC server/client 与测试。
5. 更新 `docs/PROTOCOL.md` 与相关 ADR/CHANGELOG。

## Verify

- 运行 `make verify-proto`。
- 运行 `make verify-proto-break`。
- 最后运行 `make verify-fast`；协议面较大时运行完整 `make verify`。
