# lsp-server

基于 Go 的**自研**麻将游戏服务器。

## 阶段

- Phase 0：工程治理基线。
- Phase 1：单进程川麻血战到底 MVP。
- Phase 2：`gate` / `lobby` / `room` 拆分、etcd + Redis 集群基线。
- Phase 3：PostgreSQL 事件日志、Redis 会话与快照元数据、登录重连、`SnapshotRoom`/`StreamEvents` 游标重放、Prometheus 指标与健康检查（可选 `obs.addr`）。
- Phase 4：交互式房间主循环基线，补齐 `HeartbeatReq` / `LeaveRoomReq`、换三张/定缺确认链路，以及碰/杠多候选抢答提示、确定性裁决与托管推进。
- Phase 5：血战规则补完、room 引擎拆分、proto baseline 重置、可注入时钟、双层限流、WS 幂等与最小可观测指标集合。
- Phase 5.3 / 5.4 / 5.5：规则深化、庄家与高阶番种、运行时参数与存储弹性。
- Phase 6：生产部署、SLO、压测与容量基线（范围见 [docs/adr/0023](docs/adr/0023-scope-and-roadmap.md)，部署/SLO 与容量见 [docs/adr/0024](docs/adr/0024-deployment-and-slo.md)、[docs/adr/0025](docs/adr/0025-load-and-capacity.md)，备份、凭据、跨地域评估与签名提交见 [docs/adr/0026](docs/adr/0026-postgres-backup-and-restore.md)、[docs/adr/0027](docs/adr/0027-secret-and-credential-management.md)、[docs/adr/0028](docs/adr/0028-multi-region-topology.md)、[docs/adr/0029](docs/adr/0029-signed-commit-required.md)，其中 ADR-0028 仍为草案）。新规则集议题暂缓。

## 快速启动

### 本地单进程冒烟

1. 可选：复制 `configs/dev.yaml` 并按需修改监听地址。
2. 执行：`LSP_CONFIG=configs/dev.yaml go run ./cmd/all`
3. WebSocket 地址：`ws://<ServerAddr>/ws`

### 玩家终端客户端

服务端启动后，可用 release 二进制或本地构建的 `lsp-cli` 连接 `gate` 的 WebSocket 地址：

```bash
make build-cli
./dist/lsp-cli --ws wss://racoo.cn/ws --name "我自己"
```

`lsp-cli` 先进入登录页，再进入大厅页，可刷新房间列表、自动匹配、创建公开/私密房，入座后使用俯视牌桌 TUI。默认 ASCII 牌面保证 SSH 与常见终端对齐，中文牌面可通过 `--cjk-tiles` 开启。更多参数见 [`cmd/cli/README.md`](cmd/cli/README.md)。

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

### 单机 Docker Compose

仅适用于单机 / 小规模 / 单租户上线，详见 [ADR-0030](docs/adr/0030-single-host-compose-deploy.md) 与 [`deploy/compose/README.md`](deploy/compose/README.md)：

```bash
cd deploy/compose
cp .env.example .env

vim .env

docker compose build
docker compose up -d
```

跨副本高可用、跨地域多活请回到 Kubernetes 形态（见 [ADR-0024](docs/adr/0024-deployment-and-slo.md) 与 `deploy/k8s/`）。

协议见 [docs/PROTOCOL.md](docs/PROTOCOL.md)，集群分工见 [docs/CLUSTER.md](docs/CLUSTER.md)。

## 命令

- `make bootstrap`
- `make generate`
- `make build-cli` / `make build-cli-all`（构建当前平台或五平台 `lsp-cli` 二进制）
- `make verify`
- `RUN_INTEGRATION=1 make verify-test-integration`（执行重连、幂等、限流与托管超时集成目标）
- `SCENARIO=a make verify-bench`（独立运行 Phase 6 压测场景，不进入默认 `make verify`）
- `go test ./internal/app -run TestClusterProcessesFourPlayersReceiveSettlement -v`（跨进程四人完整回放冒烟）
- `make verify-git-repo`（仓库卫生与 hook/CI 映射；亦由 `verify` / `verify-fast` 调用）
- `make verify-pre-commit`（本地提交前：`verify-git-local` + `verify-fast`，由 `pre-commit` 调用）

Git 策略见 [docs/adr/0007-git-workflow-policy.md](docs/adr/0007-git-workflow-policy.md)。
