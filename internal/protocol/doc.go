// Package protocol 合并帧编解码与消息类型编号，实现 ADR-0003 约定的二进制帧协议。
//
// 禁止：不得依赖 session、handler、store、service 或 app 等层。
package protocol
