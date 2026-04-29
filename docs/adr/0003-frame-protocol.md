---
title: 帧协议
status: accepted
date: 2026-04-22
---

# ADR-0003 帧协议

## 状态

已采纳。

## 决策

客户端流量采用**紧凑定长二进制头** + **Protobuf** 载荷。

`msg_id` 由 `internal/net/msgid` 与 `docs/PROTOCOL.md` 共同描述；`31` 分配给 `InitialDealNotify`，用于开局后按座位定向下发当前玩家的初始手牌。定向语义不放入 9 字节帧头，而由服务端内部 `Notification.TargetSeat` / `cluster.v1.RoomServiceStreamEventsResponse.target_seat` 承载，最终仍映射成普通 WebSocket 二进制帧。

## 理由

- 在 WebSocket 二进制消息上提供稳定成帧。
- 与具体编程语言解耦。
- 兼容后续服务拆分与回放工具链。
