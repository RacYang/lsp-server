# ADR-0027 密钥与凭据管理

## 状态

已采纳。生产凭据采用云 KMS 或托管 Secret Manager 作为长期存储源，Kubernetes Secret 仅作为运行时投递形态；示例 overlay 见 `deploy/k8s/overlays/example`。

## 背景

ADR-0024 决定 Redis、PostgreSQL、etcd 凭据走 Kubernetes Secret，避免进入镜像层。
但 Secret 来源、轮换周期、应用侧读取边界尚未统一，容易在部署脚本与配置模板之间漂移。

## 决策

### 1. 凭据来源

- 生产环境优先使用云厂商 KMS 或托管 Secret Manager。
- Kubernetes Secret 只作为运行时投递形式，不作为长期存储源。
- 本地开发使用占位模板，真实凭据不得提交到 Git。

### 2. 轮换策略

| 凭据 | 轮换周期 | 备注 |
| --- | --- | --- |
| Redis | 90 天 | 支持双密钥窗口时先增后删 |
| PostgreSQL | 90 天 | 轮换前后必须跑连接池冒烟 |
| etcd | 180 天 | 若启用 mTLS，证书另立证书生命周期 ADR |

### 3. 应用读取边界

- 应用仍只读取进程 YAML；是否由 initContainer 或外部控制器渲染 Secret 到 YAML，
  属于部署层职责。
- `.build/config.yaml` 不承载任何运行时 Secret 字段。
- `make verify-secrets` 继续作为仓库层最后防线。

## 后果

- `deploy/k8s/overlays/example` 提供 Secret 渲染示例，但只保留 placeholder。
- 引入新凭据类型时必须更新本 ADR 或新增 ADR。
- 若未来采用 mTLS，需要单独讨论证书签发、吊销与续期。
