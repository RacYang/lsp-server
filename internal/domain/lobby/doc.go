// Package lobby 定义大厅领域类型，供 service/lobby 与 store/redis 共享。
//
// 职责：声明 RoomRecord 数据结构与 RoomRegistry 接口，消除 store → service 反向依赖。
// 禁止在本包内引入网络、存储或会话关切。
package lobby
