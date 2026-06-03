// Package codec 是 engine 的唯一 proto 序列化边界。
//
// 职责：将 engine 传入的基础类型数据构造为 client.v1 proto 消息并序列化为字节。
// engine 主包只依赖本包，不直接 import api/gen/go/client/v1 或 proto 库。
// 本包不得 import engine 主包（会引入循环依赖）；所有参数均为基础类型或本包自定义数据结构。
package codec
