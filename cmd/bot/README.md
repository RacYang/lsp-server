# lsp-bot 机器人客户端

`lsp-bot` 是独立机器人陪玩进程。它只通过现有 WebSocket + Protobuf 协议进入房间，服务端看到的是普通玩家连接，不需要特殊机器人座位或额外协议。

## 基本用法

```bash
go run ./cmd/bot -ws ws://127.0.0.1:8080/ws -room room-1 -count 3
```

常用参数：

- `-room`：要加入的房间 ID，必填。
- `-count`：启动的机器人数量。
- `-name-prefix`：机器人昵称前缀。
- `-strategy`：内置难度，支持 `easy`、`normal`、`hard`，默认 `normal`。
- `-think-min` / `-think-max`：覆盖思考延迟；都设为 `0` 可关闭延迟，便于测试。
- `-token-dir`：每个机器人独立保存会话 token，避免重连时互相覆盖。

## 信息边界

机器人只读取当前连接可见的协议事实：

- 自己的手牌：来自 `InitialDealNotify` 与 `SnapshotNotify.your_hand_tiles`。
- 公开牌桌：弃牌、副露、定缺、摸牌广播、抢答候选。
- 不读取其它玩家手牌，不读取牌墙具体顺序，不直接访问房间 `RoundState`。

因此机器人基于不完美信息推理，公平性与真人客户端一致。

## 难度

- `easy`：弱启发式，主要按缺门与简单排序出牌。
- `normal`：默认档，使用向听数、进张数量、剩余张数与简单安全度综合决策。
- `hard`：在 `normal` 基础上更积极碰杠，并偏向高价值路线。

LLM 接入目前仅保留 `Advisor` 接口与 build tag 占位，默认构建不引入外部 SDK 或真实网络调用。
