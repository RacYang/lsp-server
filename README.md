# lsp-server

基于 Go 的**自研**麻将游戏服务器。

## 阶段

- Phase 0：工程治理基线。
- Phase 1：单进程川麻血战到底 MVP。
- Phase 2：基于 Redis 的拆分服务。
- Phase 3+：持久化、重连、可观测性与更多规则集。

## 快速启动（Phase 1）

1. 可选：复制 `configs/dev.yaml` 并按需修改监听地址。
2. 终端执行：`LSP_CONFIG=configs/dev.yaml go run ./cmd/all`（未设置 `LSP_CONFIG` 时默认仍尝试加载该路径）。
3. WebSocket 地址：`ws://<ServerAddr>/ws`，协议见 [docs/PROTOCOL.md](docs/PROTOCOL.md)。

## 命令

- `make bootstrap`
- `make generate`
- `make verify`
- `make verify-git-repo`（仓库卫生与 hook/CI 映射；亦由 `verify` / `verify-fast` 调用）
- `make verify-pre-commit`（本地提交前：`verify-git-local` + `verify-fast`，由 `pre-commit` 调用）

Git 策略见 [docs/adr/0007-git-workflow-policy.md](docs/adr/0007-git-workflow-policy.md)。
