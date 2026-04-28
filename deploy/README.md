# 部署工件

`deploy/` 收纳 Phase 6 生产交付相关工件，和业务代码保持隔离。

- `docker/`：`gate`、`room`、`lobby` 的多阶段镜像定义。
- `k8s/`：Kubernetes 基础清单与配置模板（[ADR-0024](../docs/adr/0024-deployment-and-slo.md) 默认生产形态）。
- `compose/`：单机 Docker Compose 部署形态（[ADR-0030](../docs/adr/0030-single-host-compose-deploy.md)，与 K8s 并存，限定单机 / 小规模 / 单租户）。
- `observability/`：Prometheus recording rule 与 alerting rule。
- `ops/`：PostgreSQL 备份恢复演练 runbook。
- `release/`：灰度发布与回滚脚本（仅适用于 K8s 形态）。

镜像构建只在本地或发布流水线中执行，不进入默认 `make verify` 链路。
K8s Secret overlay 示例位于 `k8s/overlays/example/`，只保留 placeholder；
Compose 形态凭据来源见 `compose/README.md`，真实凭据由部署侧 `.env` 注入并不得入 Git。
