# 部署工件

`deploy/` 收纳 Phase 6 生产交付相关工件，和业务代码保持隔离。

- `docker/`：`gate`、`room`、`lobby` 的多阶段镜像定义。
- `k8s/`：Kubernetes 基础清单与配置模板。
- `observability/`：Prometheus recording rule 与 alerting rule。
- `ops/`：PostgreSQL 备份恢复演练 runbook。
- `release/`：灰度发布与回滚脚本。

镜像构建只在本地或发布流水线中执行，不进入默认 `make verify` 链路。
Secret overlay 示例位于 `k8s/overlays/example/`，只保留 placeholder，真实凭据由 KMS 或托管 Secret Manager 投递。
