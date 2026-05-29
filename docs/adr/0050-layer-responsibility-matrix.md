---
title: 各层职责矩阵与包边界约束
status: accepted
date: 2026-05-29
---

# ADR-0050 各层职责矩阵与包边界约束

## 背景

全项目扫描发现统一根因：每一层都在做不属于自己的事，且没有硬约束阻止持续蔓延。`handler` 承载限流/幂等、`adapter/local` 承载会话恢复、`service/room` 同时是类型仓库和状态机，导致跨层类型依赖与职责漂移。ADR-0008 定义了进程边界，本 ADR 定义进程内各 Go 包的职责边界与类型归属。

## 决策

### 1. 七层模型

| 层编号 | 名称 | Go 包路径 | 职责一句话 |
|--------|------|-----------|------------|
| L7 | transport | `internal/handler` | WebSocket 帧收发、帧路由到 gateway、限流入口 |
| L6 | gateway | `internal/adapter/*`、`internal/gateway/remote` | 协议转换（proto↔Go 类型）、节点路由、通知序列化 |
| L5 | contract | `internal/contract` | 跨层共享接口定义（RoomGateway、ResumeResult） |
| L4 | service | `internal/service/room`、`internal/service/lobby` | 并发编排（actor 生命周期）、房间/大厅状态管理 |
| L3 | engine | `internal/service/room/engine`（待建） | 确定性游戏状态机、RoundState 变更、通知生成 |
| L2 | domain | `internal/domain/*` | 纯领域类型（Room FSM、Seat、PlayerProfile） |
| L1 | store | `internal/store/*`、`internal/session` | 持久化抽象（Redis/PG 读写）与连接注册表 |

跨层横切：`internal/mahjong/*`（纯规则）、`internal/cluster`（路由）、`internal/protocol`（帧编码）、`internal/metrics`、`internal/clock`、`internal/config`、`pkg/logx`。

### 2. 各层入口/出口契约与禁止事项

#### L7 transport

- 入口：原始 WebSocket 帧。
- 出口：反序列化后的 Go 命令对象，交给 L6。
- 禁止：实现业务规则、维护游戏状态、直接调用 `store`。
- 当前违反：限流逻辑（`ratelimit.go`）应下沉至 L6 adapter 中间件。

#### L6 gateway / adapter

- 入口：L7 命令对象 + L4 通知事件。
- 出口：L4 接口调用结果序列化为 proto/frame；L4 通知推送给 L7 hub。
- 禁止：持有游戏状态、执行会话重建业务逻辑、直接调用 `engine`。
- 当前违反：`adapter/local/gateway.go::Resume` 承载会话恢复完整逻辑，应拆至独立 `ReconnectService`。

#### L5 contract

- 仅含接口定义与轻量 DTO（ResumeResult、ResumeError），无实现代码。
- 禁止：import 任何业务包或基础设施包。

#### L4 service

- 入口：来自 L6 的命令（经 CommandHandler 接口）。
- 出口：`[]Notification`、`RoundView`（L3 engine 类型）。
- 禁止：序列化 proto 消息、知道 WebSocket/Hub 存在、创建 OS timer（委托给 scheduler）。
- 当前违反：`service.go::startActorLocked` 已通过 `InjectRecoveryRuntime` 修正；无其他违反。

#### L3 engine（待建子包）

- 入口：`Apply*(ctx, *RoundState, ...)` 调用。
- 出口：`([]Notification, error)`；`RoundState` 通过 accessor 方法对外只读暴露。
- 禁止：goroutine、timer、网络、持久化、import L4/L6/L7 任何包。
- 类型归属：`Notification`、`Kind`、`RoundView`、`RoundProgress`、`PhaseToken`、`WaitingReason`、`Phase`、`PhaseDriftError` 的权威定义在 L3 engine。

#### L2 domain

- 禁止：import 任何基础设施包（store、session、cluster、metrics）。

#### L1 store / session

- 禁止：import L4/L6/L7；不实现业务逻辑。

### 3. 导入方向（单向 DAG）

```text
L7 → L6 → L4 → L3 → L2
          L4 → L1
L6 → L1
L6 → L5
L7 → L5
横切层（mahjong/cluster/protocol/metrics/clock/config/logx）可被任意层 import，自身不 import L3+。
```

禁止逆向：任何 Li 不得 import Lj（j > i），唯一例外是 `contract`（L5）被 L7 和 L6 共同依赖。

### 4. 类型归属规则

- 引擎生成的类型（Notification、Kind、RoundView 系列、PhaseToken、WaitingReason）→ **L3 engine**。
- 服务层接口（CommandHandler、RoomQueries、RoomRecovery、RoomService）→ **L4 service/room**。
- 跨 gate/room 网关契约（RoomGateway、ResumeResult）→ **L5 contract**。
- WebSocket 帧 ID 与 proto 映射 → **L6 adapter** 内部，不对外暴露。
- 领域 ID 类型（Seat、RoomID、UserID）→ **L2 domain**。
- 同一类型不得在两个层中重复定义（`clusterPhaseTokToRoom` 与 `PhaseTokenFromProto` 并存已于提交 008ac28 修复，此为参照样板）。

### 5. 当前违反清单与迁移优先级

| 违反 | 所在文件 | 正确落点 | 优先级 |
|------|----------|----------|--------|
| 限流策略在 L7 handler | `handler/ratelimit.go` | L6 adapter 中间件 | P1 |
| 会话恢复逻辑在 L6 adapter | `adapter/local/gateway.go::Resume` | 独立 `service/reconnect` 或 session 层 | P1 |
| engine 类型归属 L4（未建子包） | `service/room/engine.go` 等 | `service/room/engine/` 子包 | P1 |
| `service/room` actor+engine 同包 | `actor.go`、`engine.go` | actor 子包与 engine 子包分离 | P1 |
| anyProjectDeps 剩余 5 个组件 | `.go-arch-lint.yml` | 逐个补 deny 规则（app/cluster/config/log/cmd） | P2 |
| `handler` mayDependOn 列表过宽 | `.go-arch-lint.yml` | 收缩至 contract/session/protocol/config/log | P2 |

### 6. 迁移阶段

- **Phase A（本 ADR）**：写定职责矩阵，不动代码。
- **Phase B**：建 `service/room/engine/` 子包，迁移 engine_*.go + phase.go + view_types.go + 相关类型；外部调用方 import 路径从 `service/room` 改为 `service/room/engine`（仅数据类型部分）。
- **Phase C**：建 `service/room/actor/` 子包，迁移 actor.go + actor_dispatch.go + scheduler.go。
- **Phase D**：将 `Resume` 逻辑从 `adapter/local` 分离至 `service/reconnect`。
- **Phase E**：将 `handler` 限流下沉至 `adapter` 层；收缩 arch-lint 规则。

每个 Phase 独立提交，Phase B 是高优先级起点。

## 后果

- 新增 Go 包只允许在 L3 engine 定义通知/视图类型；L6 不再"借用" `service/room` 包。
- `internal/handler` 的 `mayDependOn` 可以收缩到 4 个组件，强制 transport 层轻量化。
- Phase B 完成后，`service/room` 包大小从 ~8800 行降至 ~2000 行。
- `adapter/room` 的 `clientv1` import 数量减少（通知/视图类型来自 engine，不再需要 service/room 全包）。
- 迁移期间外部调用方需更新部分 import 路径，但接口（CommandHandler/RoomGateway）不变。
