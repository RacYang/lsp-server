---
title: Phase 4 交互式房间事件循环与最小恢复边界
status: accepted
date: 2026-04-24
---

# ADR-0015 Phase 4 交互式房间事件循环与最小恢复边界

## 状态

已采纳。

## 背景

Phase 3 之前，房间在四人 `ready` 之后会直接自动回放整局并输出结算。这种实现虽然能覆盖换三张、定缺、结算与事件流链路，但无法支撑真实客户端交互，也无法在 `room` 重启后继续一局尚未结束的牌。

Phase 4 需要同时解决三个问题：

1. 客户端动作必须能真正进入 `ws -> gate -> room.ApplyEvent -> actor -> StreamEvents` 主链路。
2. 房间运行态必须显式区分“等谁出牌”“是否可自摸”这类局内等待态。
3. 冷启动恢复不能只停留在“房间还在 playing”，而要至少能继续处理下一步真实输入。

## 决策

### 1. 运行态放在 `internal/service/room.RoundState`

- `domain/room.Room` 仍只维护房间级聚合信息（玩家、准备、FSM）。
- 具体牌局运行态放在 `RoundState`，由 room actor 串行持有：
  - 四家手牌。
  - 牌墙剩余顺序。
  - 当前轮到谁。
  - `waiting_discard` / `waiting_tsumo`。
  - 定缺、累计番数、赢家座位。

这样可以避免把大量局内细节塞回 `domain/room`，同时保持“每房单协程修改单一运行态”的模型。

### 2. 动作链路按“最小闭环优先”落地

- `discard_req`：完整打通并真正推进轮次。
- `hu_req`：当前仅覆盖自摸待决窗口；客户端看到 `action = "tsumo_choice"` 后可选择胡或直接打出摸到的牌。
- `pong_req` / `gang_req`：支持针对最近一次弃牌的多候选抢答窗口，可中断当前待出牌座位；Phase 4 的裁决为“杠优先于碰、同优先级按出牌座位下家顺序”。Phase 5 起该优先级扩展为“胡优先于杠、杠优先于碰”。

### 3. 恢复只承诺“最小可继续交互”

Redis `snapmeta.round_json` 保存最小恢复事实集合：

- 当前轮次与等待态。
- 最近一次弃牌与当前可中断抢答窗口。
- 四家手牌。
- 牌墙剩余顺序。
- 定缺结果。
- 当前赢家 / 累计番数。

恢复目标不是完整重建所有历史并发窗口，而是保证房间重启后仍能继续：

- 接受当前轮次的 `discard_req`。
- 在自摸待决时接受 `hu_req` 或继续 `discard_req`。
- 在最近一次弃牌仍可被中断时，恢复抢答候选窗口并接受合法的 `pong_req` / `gang_req`。
- 继续通过 `StreamEvents` 向 gate 补发后续事件。

### 4. Phase 5 客户端意图消费

Phase 5 起，换三张与定缺请求中的客户端意图进入服务端裁决：

- `ExchangeThreeReq.direction` 采用 `1=顺时针`、`2=对家`、`3=逆时针`，四家必须提交一致方向；超时托管沿用默认逆时针方向，保持 Phase 4 行为。
- `QueMenReq.suit` 在 `0..2` 范围内时优先生效；只有非法值或超时托管才回退到服务端 `chooseQueSuit` 启发式。
- 玩家可自行选择手中持有量较大的花色作为缺门，服务端不拒绝该选择。该策略保留玩家自由度，后续可通过指标观察是否需要产品层约束。

### 5. 玩家客户端私有视图

Phase 6 起，`RoundState` 在开局发牌后为四个座位各生成一条 `InitialDealNotify`，通过 `Notification.TargetSeat` 定向到对应玩家。广播通知沿用 `TargetSeat = -1`；集群模式下该字段映射为 `cluster.v1.RoomServiceStreamEventsResponse.target_seat`，由 `gate` 按本地座位映射发送到目标用户连接。

`RoundView` 同时暴露四家手牌、弃牌与副露摘要；`SnapshotNotify` 只填充当前玩家自己的 `your_hand_tiles`，但完整返回 `discards_by_seat` 与 `melds_by_seat`。这样真实客户端不再依赖服务端托管的 `chooseExchangeTiles` fallback，也能在重连后恢复可继续操作的牌桌。

## 后果

- 端到端测试从“只等结算”改为“在收到 `draw_tile` 后由对应座位回发 `discard_req`”。
- `room` 冷启动恢复能力显著增强，`pong` / `gang` 的抢答候选窗口已纳入最小恢复承诺。
- 超时策略目前提供 actor 内串行执行的托管入口；后台定时器调度可在此基础上接入。
- 玩家客户端可以在不读取服务端内部状态的前提下获得自己的完整手牌；其它玩家手牌仍不通过广播泄露。
