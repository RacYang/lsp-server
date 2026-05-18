# 前端启动方式

`lsp-server` 的玩家前端只有一个：`lsp-cli` 终端 TUI。源码在 `cmd/cli`，但启动时不要进入 `cmd/cli`，也不要从 `/tmp` 启动。

统一入口：始终在仓库根目录执行。默认连接远端 `racoo` 服务器；本地单进程服务端只作为后备开发方式。

## 远端服务器

默认试玩和前端联调走远端服务：

```bash
make build-cli
./dist/lsp-cli --ws wss://racoo.cn/ws --name "我自己"
```

如果要直接在服务器 SSH 会话里玩，先进入服务器：

```bash
ssh racoo
```

然后在服务器上的仓库根目录执行同一套前端入口：

```bash
make build-cli
./dist/lsp-cli --ws wss://racoo.cn/ws --name "我自己"
```

如果 `~/.lsp/config.toml` 已保存过服务器地址和昵称，可以直接：

```bash
./dist/lsp-cli
```

## 本地后备

只有在明确需要本地后端排查时，才启动本地单进程服务端：

```bash
LSP_CONFIG=configs/dev.yaml go run ./cmd/all
```

再开一个终端，从仓库根目录构建并启动前端：

```bash
make build-cli
./dist/lsp-cli --ws ws://127.0.0.1:18080/ws --name "我自己"
```

## 发布包

`scripts/lsp-cli.sh` 和 `scripts/lsp-cli.ps1` 只作为 release archive 的启动包装器使用。开发时优先使用 `make build-cli` 生成的 `./dist/lsp-cli`。

## 路径边界

- `cmd/cli`：前端源码目录，不是启动目录。
- `dist/lsp-cli`：本地构建后的前端二进制，是开发启动入口。
- `scripts/lsp-cli.sh` / `scripts/lsp-cli.ps1`：发布包入口。
- `/tmp`：只允许作为测试、帧 dump 或临时文件目录，不是前端启动目录。

## 测试边界

- 前端手工测试默认连接远端 `racoo` 服务器。
- 不再默认启动本地 `cmd/all`；除非任务明确要求排查本地后端。
