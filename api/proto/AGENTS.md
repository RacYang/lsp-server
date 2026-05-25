# api/proto

proto 契约定义层。客户端消息契约（`client/v1`）与服务间 RPC 契约（`v1`）共用同一 proto 模块。

## 结构

- `client/v1/messages.proto`：客户端可见消息类型（`SeatInfo`、`Phase`、`Envelope` 等）。
- `v1/service.proto`：集群内部 RPC 接口（`RoomService`、`LobbyService`），直接引用 `client/v1` 消息。

## 硬约束

- 在既有 message、oneof、enum 中只追加字段编号；删除或复用编号必须先走新版本包或独立 ADR。
- `make verify-proto-break` 基于 `proto-baseline` tag 执行 buf breaking 检查，不得跳过。
- `package` 与 `go_package` 选项不得在无版本 bump 的情况下改动。
- 运行 `make generate` 后把生成产物（`api/gen/go`）一起提交，禁止手改生成文件。

## 相关

- **ADR**：`docs/adr/0012-proto-baseline-and-versioning.md`
- **协议文档**：`docs/PROTOCOL.md`
