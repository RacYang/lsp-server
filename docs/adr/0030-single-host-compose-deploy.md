---
title: 单机 Docker Compose 部署形态
status: accepted
date: 2026-04-28
---

# ADR-0030 单机 Docker Compose 部署形态

## 状态

已采纳。本 ADR 与 [ADR-0024](0024-deployment-and-slo.md) 并存：[ADR-0024](0024-deployment-and-slo.md) 是默认生产形态（Kubernetes），本 ADR 仅适用于「单机 / 小规模 / 单租户」场景，不替代 [ADR-0024](0024-deployment-and-slo.md)。

## 背景

[ADR-0024](0024-deployment-and-slo.md) 把 Kubernetes 钉为默认生产形态，并在 §5 明确「任何未来引入新部署形态（如 ECS、Nomad）需补独立 ADR，避免 drift」。在小规模上线、私有部署或单机演示等场景下：

1. 没有 Kubernetes 控制面与发布流水线，[ADR-0024](0024-deployment-and-slo.md) 的 `deploy/release/canary.sh` 流程无法套用。
2. Redis、PostgreSQL、etcd 三类依赖项需要在同机部署并随服务一起编排，[ADR-0024](0024-deployment-and-slo.md) §1.2 的「StatefulSet 或托管」二选一路径都不适用。
3. 凭据来源仍须遵守 [ADR-0027](0027-secret-and-credential-management.md)：应用只读取进程 YAML，部署层负责把凭据渲染进 YAML。
4. SLO 在 [ADR-0024](0024-deployment-and-slo.md) 中是基于 Kubernetes 多副本 + 监控大盘的对外承诺；单机 Compose 形态不具备相同的高可用与告警能力，不能直接套用。

为避免 Compose 形态与既有 ADR 漂移，本 ADR 给出独立决策。

## 决策

### 1. 适用边界

Compose 形态仅适用于以下场景：

- 单机或同机房小规模上线（玩家并发 ≤ 单机基线测算容量，详见 [ADR-0025](0025-load-and-capacity.md)）。
- 私有化或单租户交付，不需要跨副本高可用。
- 演示、预发、客户 PoC。

**不适用** 于跨节点高可用、跨地域多活、大规模生产。后两类场景必须沿用 [ADR-0024](0024-deployment-and-slo.md)（[ADR-0028](0028-multi-region-topology.md) 的多地域议题不在本 ADR 范围）。

### 2. 拓扑

Compose 形态采用「拟生产三进程 + 同机依赖」拓扑：

| 容器 | 用途 | 端口 | 持久化 |
| --- | --- | --- | --- |
| `gate` | 客户端 WebSocket 接入 | `18080` WS、`18081` obs | 否 |
| `room` | 房间 actor 与持久化写入 | `19082` gRPC、`19083` obs | 否 |
| `lobby` | 大厅 / 房间分配 | `19081` gRPC、`19084` obs | 否 |
| `redis` | 会话、幂等、快照元数据 | `6379` | named volume |
| `postgres` | 事件日志与结算 | `5432` | named volume |
| `etcd` | 控制面（单节点） | `2379` | named volume |
| `gate-config-render`、`room-config-render`、`lobby-config-render` | 一次性 YAML 渲染容器 | 无 | 共享 named volume |

应用容器镜像沿用 [ADR-0024](0024-deployment-and-slo.md) §1.1 的 distroless 基线，**不得**为 Compose 形态偏离镜像约束。

### 3. 凭据与配置渲染

- 凭据来源：根目录 `.env`（已在 `.gitignore` 跟踪范围之外），由 `deploy/compose/.env.example` 提供占位模板。
- 渲染机制：每个应用容器配一个 `*-config-render` 边车容器，使用 `alpine:3.20` + `envsubst`，把 `deploy/compose/configs/*.yaml.template` 中的 `${POSTGRES_PASSWORD}` 等占位替换为真实值，落到共享 named volume，应用容器只读挂载。
- 应用读取边界遵循 [ADR-0027](0027-secret-and-credential-management.md) §3：进程仍只读 `/etc/lsp/*.yaml`，不感知凭据来源。
- 真实 `.env` 不进 Git；`make verify-secrets` 与 `gitleaks` 继续作为最后防线。

### 4. 数据持久化与备份

- Redis、PostgreSQL、etcd 三类依赖均使用 Docker named volumes（`lsp_redis_data`、`lsp_postgres_data`、`lsp_etcd_data`），不使用 bind mount，避免误删或权限漂移。
- PostgreSQL 备份与恢复仍按 [ADR-0026](0026-postgres-backup-and-restore.md)：Compose 形态额外补充 `deploy/compose/ops/postgres-dump.sh` 占位，作为单机版每日全量入口；运维侧负责把备份转存到对象存储，仓库内不保留备份产物。
- Compose 形态下 RPO/RTO 沿用 [ADR-0026](0026-postgres-backup-and-restore.md) 目标；若实际部署不具备 WAL 归档能力，必须在 Compose 形态运行手册中显式记录降级。

### 5. 健康检查与可观测

- 三应用容器在 Compose 中声明 `healthcheck`，HTTP `GET /healthz` on `obs.addr`；`gate` 额外声明 `readyz` 作为 ready gate。
- 不内置 Prometheus / Alertmanager；指标暴露口仍为 `/metrics`，由部署侧选择是否接入外部监控。
- [ADR-0024](0024-deployment-and-slo.md) 的对外 SLO（抢答完成率、重连成功率、结算延迟 p99）**不作为 Compose 形态的对外承诺**。Compose 形态把这些指标定位为「可观测内部健康度」，不进入产品级 SLA。

### 6. 发布与回滚

- Compose 形态的发布通过镜像 tag 切换（`docker compose pull && docker compose up -d`）实现，沿用 [ADR-0024](0024-deployment-and-slo.md) §4「禁止原地修代码 hotfix」的纪律。
- 不引入金丝雀分级；单机部署没有副本可放量。回滚直接切回上一镜像 tag。
- 镜像 tag 仍须遵守 `git.tags.release_pattern` 与 [ADR-0029](0029-signed-commit-required.md) 的签名校验试运行；签名强制升级为 `required` 后，Compose 形态须同步引入 tag 校验脚本。

### 7. 工程边界

- 不修改 `.build/config.yaml`、不新增 enforcer、不改业务代码与 [ADR-0024](0024-deployment-and-slo.md) 镜像约束。
- 不引入新的 Cursor 规则文件；Compose 形态由 `deploy/compose/README.md` 承担运维手册职责。
- 不接入 `make verify` 链路；`make verify-secrets` 已能扫描整个工作树，足以覆盖凭据回归。
- 与 [ADR-0008](0008-cluster-topology-control-data-plane.md) 的关系：Compose 形态使用单节点 etcd，违背 [ADR-0008](0008-cluster-topology-control-data-plane.md) 「etcd ≥3 节点」的生产建议，但本 ADR §1 已限定不适用于高可用场景，单节点 etcd 仅作为本地控制面占位。

## 后果

- `deploy/compose/` 落地 Compose 编排、`.env.example`、配置模板、渲染容器与运维 README。
- `deploy/k8s/` 与 `deploy/release/` 不做任何变更，[ADR-0024](0024-deployment-and-slo.md) 路径完整保留。
- 当 Compose 形态出现新需求（如多节点、跨机房、Swarm 模式），必须先补独立 ADR，不得在 `deploy/compose/` 下隐式扩张。
- 若 Compose 形态长期承担生产流量，须在 follow-up ADR 中评估是否回归 Kubernetes 形态或扩展本 ADR 的拓扑约束。
