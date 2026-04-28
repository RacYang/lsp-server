# 单机 Docker Compose 部署

本目录承载 [ADR-0030](../../docs/adr/0030-single-host-compose-deploy.md) 定义的单机 / 小规模 / 单租户部署形态。
与 [ADR-0024](../../docs/adr/0024-deployment-and-slo.md) Kubernetes 形态并存，不替代。

## 适用边界

- 同机房单机上线、私有化或单租户交付、演示与 PoC。
- 不承担 [ADR-0024](../../docs/adr/0024-deployment-and-slo.md) 的对外 SLO；不具备跨副本高可用。
- 跨节点高可用、跨地域多活请回到 Kubernetes 形态。

## 拓扑

| 容器 | 镜像 | 网络位置 | 备注 |
| --- | --- | --- | --- |
| `gate` | 本地 build `lsp-gate` | 仅 WebSocket 端口暴露宿主机 | 沿用 distroless 镜像 |
| `room` | 本地 build `lsp-room` | 仅 obs 端口绑定 `127.0.0.1` | 沿用 distroless 镜像 |
| `lobby` | 本地 build `lsp-lobby` | 仅 obs 端口绑定 `127.0.0.1` | 沿用 distroless 镜像 |
| `redis` | `redis:7-alpine` | 仅 compose 内网 | named volume 持久化 |
| `postgres` | `postgres:16-alpine` | 仅 compose 内网 | named volume 持久化 |
| `etcd` | `bitnami/etcd:3.5` | 仅 compose 内网 | 单节点；不满足 ADR-0008 高可用建议 |
| `config-render` | `alpine:3.20` | 一次性 | envsubst 渲染 YAML 到共享卷 |

## 初次启动

```bash
cd deploy/compose
cp .env.example .env

vim .env

docker compose build

docker compose up -d
```

`docker compose up` 会按以下顺序拉起：

1. `redis` / `postgres` / `etcd` 启动并通过 healthcheck。
2. `config-render` 以 `alpine:3.20` 安装 `gettext`，将 `configs/*.yaml.template` 中的 `${POSTGRES_USER}` 等占位渲染到 `lsp_config` named volume，写完即退出。
3. `lobby` / `room` / `gate` 顺序启动；它们以只读方式挂载 `lsp_config` 至 `/etc/lsp`，应用仍只读 YAML，不感知凭据来源（沿用 [ADR-0027](../../docs/adr/0027-secret-and-credential-management.md) §3）。

健康验证：

```bash
docker compose ps
curl -fsS http://127.0.0.1:18081/healthz
curl -fsS http://127.0.0.1:18081/readyz
```

客户端 WebSocket 直连 `ws://<host>:18080/ws`。

## 凭据来源

- 真实凭据写入 `deploy/compose/.env`，**不入 Git**（仓库根 `.gitignore` 已排除）。
- `make verify-secrets` 会扫描整个工作树，提交前必须为空。
- 凭据轮换沿用 [ADR-0027](../../docs/adr/0027-secret-and-credential-management.md) §2：PostgreSQL 90 天、Redis 90 天、etcd 180 天；轮换后 `docker compose up -d --force-recreate config-render <服务>`。

## 数据持久化与备份

- Redis、PostgreSQL、etcd 全部使用 named volumes：`lsp_redis_data` / `lsp_postgres_data` / `lsp_etcd_data`。`docker compose down` 不删除卷；`docker compose down -v` 会清空数据，请谨慎使用。
- PostgreSQL 备份占位脚本：

```bash
BACKUP_DIR=/var/backups/lsp \
POSTGRES_USER=lsp \
POSTGRES_DB=lsp \
bash deploy/compose/ops/postgres-dump.sh
```

按 [ADR-0026](../../docs/adr/0026-postgres-backup-and-restore.md) 周期把产物转存到对象存储或异地磁盘；恢复演练继续沿用 `deploy/ops/postgres-restore.md`。

## 升级与回滚

- 升级：`docker compose pull && docker compose up -d`，或本地 `docker compose build --no-cache && docker compose up -d`。
- 回滚：把 `.env` 中 `*_IMAGE_TAG` 切回上一个已签名 tag（[ADR-0029](../../docs/adr/0029-signed-commit-required.md) 升级为 `required` 后须强制校验签名），再 `docker compose up -d`。
- 禁止「原地修代码 hotfix」（沿用 [ADR-0024](../../docs/adr/0024-deployment-and-slo.md) §4 纪律）。
- Compose 单机形态不分级灰度；如需灰度请回到 Kubernetes 形态。

## 与 SLO 的关系

- 抢答完成率、重连成功率、结算延迟 p99 在 Compose 形态下作为「内部健康指标」，**不构成对外 SLA**（[ADR-0030](../../docs/adr/0030-single-host-compose-deploy.md) §5）。
- 指标仍通过 `/metrics` 暴露；运维侧可自行接入外部 Prometheus / Grafana。

## 已知局限

- 单节点 etcd：不满足 [ADR-0008](../../docs/adr/0008-cluster-topology-control-data-plane.md) 「≥3 节点」建议，仅作为本地控制面占位。
- 无 WAL 归档：默认 `postgres:16-alpine` 不启用归档，RPO 可能差于 [ADR-0026](../../docs/adr/0026-postgres-backup-and-restore.md) 目标；如需达成 RPO ≤ 15 分钟，请在 `postgres` 服务上额外挂载 `archive_command` 配置。
- 无对外告警：本 Compose 不内置 Prometheus / Alertmanager；外部监控由部署侧自行接入。
