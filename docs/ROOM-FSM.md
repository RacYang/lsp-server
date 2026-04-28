# 房间 FSM（Phase 5/6）

## 状态

- `idle`：占位，当前实现从 `waiting` 起步。
- `waiting`：等待玩家进房与准备。
- `ready`：四人已满且全准备，可开局。
- `playing`：对局进行中。房间不再自动整局回放，而是进入真实交互循环：
  - 广播换三张、定缺、开局。
  - 当前座位摸牌后进入 `等待出牌`。
  - 若摸牌立即可自摸，则先进入 `等待自摸决策`，客户端可发送 `hu_req` 或对摸到的牌发送 `discard_req`。
  - 最近一次弃牌会保留一个抢答窗口；可抢座位收到 `hu_choice` / `pong_choice` / `gang_choice` / `qiang_gang_choice` 后，可发送对应请求中断当前待出牌座位。
  - 玩家胡牌后记录 `hued_seats` 并退出后续轮转；牌局到三家胡或牌墙耗尽时进入结算。
- `settling`：结算中。
- `closed`：房间关闭。

## 迁移

详见 `internal/domain/room/fsm.go` 中的显式迁移表；非法迁移会返回错误，避免静默破坏房间一致性。

## 超时策略

- `waiting`：超时策略仍为工程占位，当前不自动踢人；运维侧可直接回收长时间空房。
- `ready`：若未凑齐四人全准备，房间停留在 `ready` 前的等待阶段；当前不自动回退准备态。
- `playing`：
  - 出牌/自摸待决超时时，服务端托管入口可按确定性弃牌策略推进。
  - 抢答窗口超时时，服务端托管入口按“胡优先于杠、杠优先于碰、同优先级按出牌座位下家顺序”选择候选动作。
  - 后台定时器通过可注入 `clock.Clock` 调度，按配置覆盖 `exchange_three`、`que_men`、`claim_window`、`tsumo_window` 与 `discard`。

## 重连与恢复

- `SnapshotRoom` 返回房间玩家、定缺、阶段、快照游标以及等待态摘要（谁可操作、等待什么动作、候选动作）。
- `room` 冷启动可基于 `snapmeta.round_json` 恢复进行中的局面；`round_json.schema_version` 高于当前版本时降级到重新准备。
- 过牌后的 `claim_window_open=false` 不会再凭 `LastDiscard` 重新打开抢答窗口。
- 幂等仍以 `ApplyEvent.idempotency_key` 为入口，Redis 只记录请求是否已成功落地，不重放业务副作用。
- Phase 6 恢复演练以 `SnapshotRoom` 和 `StreamEvents` 可读性作为 PostgreSQL 恢复后的核心校验点。
