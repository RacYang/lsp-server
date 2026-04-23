---
name: add-pb-message
description: 为客户端或 RPC 契约新增 Protobuf 消息。用于编辑 `api/proto`、引入新消息 ID 或扩展房间与大厅 API。
---

# 新增 PB 消息

1. 将消息放在 `api/proto/client` 或 `api/proto/rpc`。
2. 保持包名与 `go_package` 值稳定。
3. 运行生成并确认生成文件已提交。
4. 仅在契约存在后再更新 handler 与帧路由。
