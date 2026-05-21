// Package room 管理牌桌生命周期：玩家加入/离开、阶段切换、动作合法性校验、
// deadline 所有权、PhaseToken 防竞态与持久化快照。
//
// 职责：房间 FSM 驱动、actor 消息循环、规则引擎调用与广播通知组装。
//
// 禁止在本包内直接访问网络传输、gRPC 集群或 Redis/Postgres 存储；
// 持久化由 engine_persist.go 通过注入的 Store 接口完成。
package room
