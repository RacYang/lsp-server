# lsp-server

基于 Go 的**自研**麻将游戏服务器。

## 阶段

- Phase 0：工程治理基线。
- Phase 1：单进程川麻血战到底 MVP。
- Phase 2：`gate` / `lobby` / `room` 拆分、etcd + Redis 集群基线。
- Phase 3：PostgreSQL 事件日志、Redis 会话与快照元数据、登录重连、`SnapshotRoom`/`StreamEvents` 游标重放、Prometheus 指标与健康检查（可选 `obs.addr`）。
- Phase 4：交互式房间主循环基线，补齐 `HeartbeatReq` / `LeaveRoomReq`、换三张/定缺确认链路，以及碰/杠多候选抢答提示、确定性裁决与托管推进。
- Phase 4+：更多规则集与运维深化。

## 快速启动

### 本地单进程冒烟

1. 可选：复制 `configs/dev.yaml` 并按需修改监听地址。
2. 执行：`LSP_CONFIG=configs/dev.yaml go run ./cmd/all`
3. WebSocket 地址：`ws://<ServerAddr>/ws`

### 本地三进程基线

1. 为 `gate`、`lobby`、`room` 分别准备配置文件，并设置：
   - `server.addr`
   - `rule.default_id`
   - `cluster.lobby_addr`
   - `cluster.room_addr`
   - 可选：`redis.addr`（`gate`/`room` 会话、幂等与快照）、`postgres.dsn`（`room` 事件与结算持久化）、`obs.addr`（各进程 `/healthz`、`/readyz`、`/metrics` 与 pprof）
2. 分别执行：
   - `LSP_CONFIG=path/to/lobby.yaml go run ./cmd/lobby`
   - `LSP_CONFIG=path/to/room.yaml go run ./cmd/room`
   - `LSP_CONFIG=path/to/gate.yaml go run ./cmd/gate`
3. 客户端连接 `gate` 的 WebSocket 地址；内部 gRPC 协作对客户端透明。

协议见 [docs/PROTOCOL.md](docs/PROTOCOL.md)，集群分工见 [docs/CLUSTER.md](docs/CLUSTER.md)。

## 命令

- `make bootstrap`
- `make generate`
- `make verify`
- `RUN_INTEGRATION=1 make verify-test-integration`（执行带 `integration` tag 的房间重启恢复回放）
- `go test ./internal/app -run TestClusterProcessesFourPlayersReceiveSettlement -v`（跨进程四人完整回放冒烟）
- `make verify-git-repo`（仓库卫生与 hook/CI 映射；亦由 `verify` / `verify-fast` 调用）
- `make verify-pre-commit`（本地提交前：`verify-git-local` + `verify-fast`，由 `pre-commit` 调用）

Git 策略见 [docs/adr/0007-git-workflow-policy.md](docs/adr/0007-git-workflow-policy.md)。
