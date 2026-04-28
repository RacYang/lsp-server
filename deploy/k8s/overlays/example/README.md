# Secret overlay 示例

本目录展示生产接入托管 Secret Manager 时的最小 overlay 形态。文件只保留 placeholder，不包含真实 Redis、PostgreSQL 或 etcd 凭据。

## 凭据来源

- 长期存储源：云 KMS 或托管 Secret Manager。
- 运行时投递：Kubernetes Secret。
- 应用读取：进程 YAML，不直接访问 KMS API。

## 轮换周期

| 凭据 | 周期 | 校验 |
| --- | --- | --- |
| Redis | 90 天 | 双密钥窗口内先增后删，确认 gate/room 会话读写正常 |
| PostgreSQL | 90 天 | 轮换前后跑连接池冒烟与 `SnapshotRoom` / `StreamEvents` 校验 |
| etcd | 180 天 | 轮换后确认服务发现注册、房间 ownership 枚举正常 |

## 使用方式

1. 由发布系统或外部控制器把托管 Secret 渲染为 `lsp-secrets`。
2. 使用 `kubectl apply -k deploy/k8s/overlays/example` 验证 overlay 结构。
3. 提交前执行 `make verify-secrets`，确认没有真实凭据进入工作树。
