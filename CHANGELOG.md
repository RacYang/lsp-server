# 变更日志

> 条目按时间倒序排列：最新阶段在上，最早条目在下。同一阶段内的条目按 ADR 编号倒序聚合。

## 未发布

### 治理与文档

- 文档：新增 [ADR-0030](docs/adr/0030-single-host-compose-deploy.md)，与 [ADR-0024](docs/adr/0024-deployment-and-slo.md) 并存，限定单机 / 小规模 / 单租户场景的 Docker Compose 部署形态。
- 部署：新增 `deploy/compose/`，含三服务 + Redis + PostgreSQL + 单节点 etcd 的 compose.yaml、`.env.example`、配置模板与一次性 envsubst 渲染容器；业务镜像保持 distroless 不变。
- 修复：将 compose 形态的 etcd 镜像由 `bitnami/etcd:3.5` 切换到上游 `quay.io/coreos/etcd:v3.5.16`，规避 Bitnami 旧镜像下架风险，命令行参数改为官方上游 flag 形态。
- 治理：统一 ADR 状态格式、Phase 5/6 索引、文档交叉引用与命名约定，补齐 bench/scripts 命名说明；重整 `bench/` 为「剧本 + 脚本 + 临时产物」，把预发基线与恢复演练归档迁入 `docs/baselines/` 与 `docs/drills/`。
- 治理：第二轮统一 rules/skills/AGENTS.md，新增 ADR 索引页、中文化总览、规则 frontmatter schema、Redis key/指标命名 enforcer 与相关负例。
- 文档：Phase 6 收尾补齐预发压测基线、PostgreSQL 恢复演练 runbook、Secret overlay 示例与签名提交试运行说明，ADR-0026/0027/0029 升级为已采纳。
- 治理：新增工作树副本目录拦截 enforcer，扩展 `git.repo_hygiene` 增加 `workspace_space_dirs_blocked` 与 `workspace_scan_excludes`，补独立规则与负例。
- 文档：补齐 ADR-0023 Phase 6 范围与路线，README 与 ARCHITECTURE 阶段同步更新到 Phase 5.5。
- 文档：新增 ADR-0026～0029，覆盖 PostgreSQL 备份恢复、密钥管理、跨地域多活与签名提交升级评估；明确未来规划默认不引入第二套规则集。

### Phase 6（[ADR-0024](docs/adr/0024-deployment-and-slo.md)、[ADR-0025](docs/adr/0025-load-and-capacity.md)）

- 压测：新增 `docs/baselines/phase6-preprod-20260428` 作为预发首轮基线记录，ADR-0024/0025 指向该执行编号。
- 运维：新增 `deploy/ops/postgres-restore.md` 与 `docs/drills/phase6-preprod-20260428`，记录 PostgreSQL 恢复演练路径与 RPO/RTO 结果。
- 凭据：新增 `deploy/k8s/overlays/example`，展示托管 Secret placeholder overlay 与 Redis/PostgreSQL/etcd 轮换周期。
- CI：新增签名提交试运行步骤，未签名提交与未受信公钥仅输出 warning，不修改 `git.signing.policy`。
- 部署：新增三服务 distroless 多阶段镜像、Kubernetes base 清单、ConfigMap/Secret 模板与灰度/回滚脚本。
- SLO：新增 Prometheus recording/alerting rules，抢答完成率、重连成功率与结算延迟 p99 作为对外承诺子集。
- 压测：新增 `cmd/loadgen` 与 `bench/scenarios/scenario_{a,b,c}`，通过 `make verify-bench` 独立触发，默认不进入 `make verify`。
- 容量：首轮本地基线将 `runtime.room.mailbox_capacity` 从 64 保守上调到 96。

### Phase 5.5（[ADR-0022](docs/adr/0022-runtime-knobs-and-storage-resilience.md)）

- 运行时参数：`runtime.gate.ws_rate_limit_*`、`runtime.gate.ws_idempotency_cache`、`runtime.room.mailbox_capacity`、`runtime.redis.idempotency_ttl` 由进程 YAML 与 `internal/config.Config` 管理，默认值保持 Phase 5 行为不变。
- 存储弹性：Redis 与 PostgreSQL 通过统一 helper 标注可重试错误、包裹超时并对幂等操作执行有限退避；非幂等写入仅加超时与错误分类。
- 指标兼容：`lsp_storage_op_seconds{store,op,result}` 保留 `ok/error` 语义；重试次数以新 counter 表达，避免破坏既有大盘。

### Phase 5.4（[ADR-0021](docs/adr/0021-dealer-and-advanced-fans.md)）

- 计分上下文：`rules.ScoreContext` 增加 `HuSeat`、`DealerSeat`、`IsOpeningDraw`、`IsDealerFirstDiscard`，房间引擎填充字段，规则包只消费结构化上下文。
- 高阶番种：新增天胡、地胡、龙七对与十八罗汉，开局窗口按胡牌动作的发生时刻判定。
- 持久化：`round_json` schema 升级到 v3，旧版本在缺少庄家与开局窗口字段时按"开局窗口已关闭"恢复。

### Phase 5.3（[ADR-0020](docs/adr/0020-rules-deepening.md)）

- 血战规则深化，新增听牌检测、查大叫精准化、score ledger 结算流水、自摸/点炮/抢杠胡分摊、杠分退税、包牌标记、`per_winner_breakdown` 客户端透传与将对/暗刻/暗杠等番种。

### Phase 5

- BREAKING：重置 `proto-baseline`；`client.v1.Envelope` 追加 `idempotency_key`，`SnapshotNotify` / `SnapshotRoomResponse` 追加 `claim_candidates`，`cluster.v1.SettlementEvent` 补齐 `seat_scores` / `penalties` / `per_winner_breakdown`，跨进程结算不再丢失结构化罚分。
- `room` 交互引擎按职责拆分，统一 `sichuanxzdd` 包名，并为 Redis `snapmeta.round_json` 增加 `schema_version`，未知未来版本恢复时降级为重新准备而非阻断 room 启动。
- 血战主链路支持胡后续行、点炮胡候选、抢杠胡窗口、胡/杠/碰优先级裁决、杠流水与更多番种上下文，删除 Phase 4 工程步数截断。
- 引入可注入 `clock.Clock`、每房定时器、Hub 心跳超时、WS token bucket、actor 有界 mailbox、WS 幂等重放去重与 `ERROR_CODE_RATE_LIMITED` 指标。
- 新增最小可观测指标集合（抢答窗口、自动托管、重连、actor 队列深度、存储耗时、限流/幂等/未知消息）与 ADR-0019；integration 目标覆盖重连、幂等、限流与 fakeClock 托管。

### Phase 4

- `room` 改为 `exchange_three -> que_men -> start_game -> draw` 显式推进；`ExchangeThreeReq` / `QueMenReq`、`HeartbeatReq` / `LeaveRoomReq` 已接入服务端；碰/杠抢答改为多候选窗口，先下发 `pong_choice` / `gang_choice` 并按确定性优先级裁决，不再先广播下一家摸牌后回滚。
- 治理：`verify` 新增 `verify-test-integration` 钩子与 CI integration 作业；`cmd/all`、`cmd/gate` 入口补充测试，覆盖率门闸不再排除 `cmd/**` 与 `internal/app/**`。

### Phase 3

- 新增 ADR-0013/0014；`client.v1` 登录重连字段与 `SnapshotNotify`；`cluster.v1.RoomService.SnapshotRoom`；`room` 可选 PostgreSQL 事件日志与结算表、Redis 快照元数据、`StreamEvents` 游标重放；`gate` Redis 会话令牌与 `Resume` 主路径；Prometheus 指标与各进程可观测性 HTTP（`/healthz`、`/readyz`、`/metrics`、pprof）。

### Phase 2

- 完整集群基线：新增 `cmd/gate`、`cmd/lobby`、`cmd/room` 三进程入口，`gate` 可通过 `cluster.v1` gRPC 与 `lobby/room` 协作；`cmd/all` 明确降级为本地 in-process 冒烟入口。
- `room` gRPC 已对接真实房间 worker；四人 `ready` 后可跑完整四川血战自动回放，并通过跨进程 WebSocket 冒烟测试验证 `gate -> lobby/room` 主链路。
- 引入 etcd 控制面与 Redis 数据面基础实现，补齐房间亲和、会话/幂等/路由缓存、`cluster.v1` 与 `client.v1` 协议扩展文档。

### Phase 1

- 单体 MVP：`cmd/all`、WebSocket 帧协议、房间与准备流、四客户端端到端结算冒烟；`proto-baseline` 标签与 `buf breaking` 基线；覆盖率门闸修复（模块路径归一化）；日志 message UTF-8 解析修正。
- 麻将算法层（万筒条）、牌墙、手牌、和牌判定、番种分解、可插拔规则与四川血战到底 MVP 规则包；JSON 夹具与架构分层修正。
- 启动：SSOT `stage` 切换为 `alpha`，后续新增业务代码将按覆盖率门槛校验。

### Phase 0

- 引导工程治理基线。
- 中文化与注释/日志治理：SSOT 三节、`verify-lang-*`、统一日志门面策略与负例。
