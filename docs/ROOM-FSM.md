# 房间 FSM（Phase 1）

## 状态

- `idle`：占位，当前实现从 `waiting` 起步。
- `waiting`：等待玩家进房与准备。
- `ready`：四人已满且全准备，可开局。
- `playing`：对局进行中（后续迭代补齐摸打与血战分支）。
- `settling`：结算中。
- `closed`：房间关闭。

## 迁移

详见 `internal/domain/room/fsm.go` 中的显式迁移表；非法迁移会返回错误，避免静默破坏房间一致性。

## TODO

- 超时踢人与准备回退策略。
- 断线重连与幂等恢复。
