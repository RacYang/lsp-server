# 术语表

- **SSOT**：单一事实源，即 `.build/config.yaml`。
- **Constraint（约束）**：可强制执行的仓库硬规则。
- **Norm（规范）**：面向人类与 Agent 的描述性指引。
- **Negative sample（负例）**：故意无效、用于证明 enforcement 的样本。
- **Rule engine（规则引擎）**：某一麻将变体的可插拔玩法契约。
- **Ding que（定缺）**：川麻要求的定花色流程。
- **Blood battle（血战）**：有玩家和牌后牌局在约定条件下继续进行。

## 命名权威表

- **PostgreSQL**：产品名使用此写法；路径、包名与指标 label 中可保留小写 `postgres`。
- **Kubernetes**：正文使用此写法；路径名保留 `deploy/k8s/`。
- **WebSocket**：协议名使用此写法；`WS` 仅用于表格、指标或代码中需要短标识的场景。
- **runbook**：操作手册统一使用英文小写。
- **proto-baseline**：Git tag 名固定为小写加连字符。
- **Phase X / Phase X.Y**：阶段名使用英文 `Phase` 加数字；除非另有 ADR，避免出现 `Phase X.Y.Z`。
