// Package lobbyadapter 将 lobby 业务服务适配为 gRPC 传输层。
//
// 本包只承担协议适配职责：将 svcv1.LobbyService RPC 请求翻译为
// lobbysvc.Service 调用，并将响应投影回 proto 消息。
// 不得包含业务逻辑，不得依赖 app 或 cmd 包。
package lobbyadapter
