# ADR-0024 Phase 6 部署形态、SLO 与灰度策略

## 状态

草案。等 Phase 6 实施计划落地、SLO 数值取得线上基线后再升级为已采纳。

## 背景

仓库已具备生产级核心能力（ADR-0008 集群拓扑、ADR-0011 房间亲和、ADR-0019 可观测最小指标、ADR-0022 运行时参数与存储弹性），但当前所有部署仍依赖本地 `go run` 与 `make verify`。把仓库交付到任何在线环境需要：

1. 容器化形态（Dockerfile / 镜像 / 入口）。
2. 编排形态（k8s manifest 或等价物）与依赖项（Redis、PostgreSQL、etcd）的拓扑。
3. SLO 与告警：在 ADR-0019 指标基础上凝练对外承诺，避免运维侧靠"看大盘"判断健康。
4. 灰度策略：发版时如何在 gate 与 room 节点上做小流量验证、回滚与冻结。

ADR-0023 已经把上述议题划入 Phase 6 范围，本 ADR 给出具体决策草案。

## 决策

### 1. 部署形态

#### 1.1 容器镜像

每个二进制（`gate`、`room`、`lobby`、`all` 仅本地用）单独出镜像：

- 基础镜像：`gcr.io/distroless/base-debian12` 或等价 minimal base，避免引入 shell。
- 构建：多阶段 Dockerfile，第一阶段 `golang:1.23-alpine`（与 `.build/config.yaml` `golang_version` 对齐）跑 `go build -trimpath -ldflags='-s -w'`，第二阶段只拷贝可执行文件与 `LICENSE`。
- 镜像 tag：`<service>-<git_short_sha>` + 语义化版本 tag（与 `git.tags.release_pattern` 一致）。
- 构建必须可重现：`-trimpath`、固定 `CGO_ENABLED=0`、`GOFLAGS='-mod=readonly'`。

#### 1.2 编排形态

k8s 是首选；不强制 k8s 之外的形态。每个服务给出独立 Deployment：

| 服务 | 副本起步 | 关键资源 | 端口 |
| --- | --- | --- | --- |
| `gate` | 2 | CPU 500m / Memory 256Mi | WebSocket、`obs.http`、gRPC |
| `room` | 2 | CPU 1 / Memory 512Mi | gRPC、`obs.http` |
| `lobby` | 1 | CPU 200m / Memory 128Mi | gRPC、`obs.http` |

依赖项采用 StatefulSet 或托管：

- Redis：单副本起步，后续按容量 ADR 决定是否升级到 Sentinel/Cluster。
- PostgreSQL：单副本起步 + 定期备份；备份策略归独立 ADR。
- etcd：≥3 节点，沿用 ADR-0008。

镜像 / 部署清单 / Helm chart 等具体文件在 Phase 6 实施计划落地时补到 `deploy/` 目录，本 ADR 不固定具体路径。

#### 1.3 配置与 Secret

- 进程 YAML 配置走 ConfigMap 挂载到 `/etc/lsp/`。
- Redis、PostgreSQL、etcd 凭据走 Secret，避免出现在镜像层；任何打包脚本必须通过 `gitleaks`（已存在于 `make verify-secrets`）。

### 2. SLO

以下 SLO 草案需在 Phase 6 实施 1～2 周后回填实测基线，再写回本 ADR：

| 维度 | 指标来源 | 目标（草案） | 告警阈值 |
| --- | --- | --- | --- |
| 房间事件循环可用性 | `lsp_actor_queue_depth` 持续高水位 + `lsp_auto_timeout_total{kind="actor_full"}` | 30 天可用率 ≥ 99.9% | actor mailbox p95 > 80% 持续 5 分钟 |
| 抢答窗口完成率 | `lsp_claim_window_total{result}` | 90 天 `result="completed"` / 总和 ≥ 99% | 5 分钟内 `result="timeout"` / 总和 > 5% |
| 重连成功率 | `lsp_reconnect_total{result}` | 30 天 `result="ok"` ≥ 99% | 5 分钟 `result="ok"` < 95% |
| 结算延迟 p99 | `lsp_storage_op_seconds{store="postgres",op="append_event"}` | p99 ≤ 200ms | p99 > 500ms 持续 5 分钟 |
| WebSocket 入站限流 | `lsp_rate_limited_total{layer="ws"}` | 占同期入站 < 0.1% | 5 分钟 > 1% |
| 未知消息率 | `lsp_unknown_msg_total` | < 0.01% | 5 分钟 > 0.1% |

SLO 的"对外承诺"从这一组中选定一个最小子集（建议：抢答完成率、重连成功率、结算延迟），其余作为内部健康指标。

### 3. 告警规则

每个 SLO 在 Prometheus 中给出对应 `recording rule` + `alerting rule`，分两级：

- `severity=warn`：5 分钟阈值越线，触发即时排障流程。
- `severity=page`：30 分钟越线或可用率燃尽预算 > 50%，触发 on-call。

告警必须包含 `service`、`room_id`（如适用）、`build_sha` 三个 label，便于回追到具体版本。

### 4. 灰度策略

发版基本流程：

1. 镜像构建后先在预发集群跑 `make verify` 与冒烟脚本（参见 ADR-0025 §1）。
2. 生产环境按"金丝雀 → 10% → 50% → 100%"四档放量，每档至少观察 30 分钟 SLO 大盘。
3. gate 与 room 必须分开放量：先 gate（影响连接层），后 room（影响业务循环）。
4. 出现下列任一情形即触发 **快速回滚**：
   - SLO 燃尽预算 > 25%。
   - `severity=page` 告警持续 5 分钟。
   - `lsp_rate_limited_total` 或 `lsp_actor_queue_depth` 出现陡升。
5. 回滚通过镜像 tag 切换实现，禁止"原地修代码 hotfix"。

### 5. 工程边界

- 本 ADR 不规定 PostgreSQL 备份 / 跨地域多活 / KMS 密钥管理；这些由后续独立 ADR 处理。
- 本 ADR 不引入新的运行时参数；运行时参数 SSOT 仍是 ADR-0022 中的 `runtime.*`。
- 本 ADR 不修改 `.build/config.yaml` 或 hook；所有部署相关变更落到 `deploy/` 与镜像构建脚本。

## 后果

- ADR 通过后，Phase 6 实施计划首批任务为：Dockerfile + k8s 清单 + recording rules + alert rules，再补灰度脚本。
- SLO 数值在线上跑稳后回写本 ADR，并把"草案"状态升级为"已采纳"。
- 任何未来引入新部署形态（如 ECS、Nomad）需补独立 ADR，避免 drift。
- 与 ADR-0019 的关系：ADR-0019 决定"采集什么"，本 ADR 决定"承诺什么"；两者通过 SLO 表对齐。
