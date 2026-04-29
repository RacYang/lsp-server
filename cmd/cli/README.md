# lsp-cli 玩家终端客户端

`lsp-cli` 是纯终端玩家客户端，连接 `gate` 的 WebSocket 地址后先进入登录页，再进入大厅页，最后渲染俯视牌桌。自己固定显示为南家，北家整行在上，东/西家夹住中央牌桌信息。

## 启动

```bash
make build-cli
./dist/lsp-cli --ws wss://racoo.cn/ws --name "我自己" --token-file ~/.lsp/session.token
```

开发时也可以继续使用 `go run ./cmd/cli`；发布包中可通过 `scripts/lsp-cli.sh` 或 `scripts/lsp-cli.ps1` 启动，默认连接 `wss://racoo.cn/ws`。

启动后在登录页确认昵称和服务器，进入大厅后可以：

- 按 `m` 自动匹配公开空房。
- 按 `n` 打开创建房间表单，可选择私密房。
- 在房间列表上按 Enter 加入选中房间。
- 输入 `join <room_id>` 手动加入私密或已知房号。

自签证书调试时可加 `--insecure-skip-verify`，该参数只用于本地排障，不建议生产使用。

## 命令

- `login [昵称]`：重新登录。
- `list` / `refresh`：刷新公开房间列表。
- `match [rule]`：自动匹配；规则为空时使用服务端默认规则。
- `create [rule] [name] [--private]`：创建房间并直接入座。
- `join <room_id>`：加入房间。
- `ready`：准备。
- `d <tile>` / `discard <tile>`：出牌，例如 `d m3`。
- `p` / `pong`：碰。协议中 `PongRequest` 不带牌，服务端按最近一次可碰弃牌隐式匹配。
- `g [tile]` / `gang [tile]`：杠；抢杠窗口可省略牌，自杠建议带牌。
- `h` / `hu`：胡。
- `ex <t1> <t2> <t3> [direction]`：换三张，方向为 `1/2/3`。
- `que <m|p|s>`：定缺，`m=万`、`p=筒`、`s=条`。
- `leave`：离房。
- `help`：打印命令摘要。
- `quit` / `q`：退出。

快捷键：

- 大厅页 `m`：自动匹配。
- 大厅页 `n`：打开创建房间表单。
- 大厅列表 Enter：加入选中房间。
- `1..9`：打出自己手牌第 1 到第 9 张。
- `0`：打出第 10 张。
- `q`：在命令栏未聚焦时退出。

## 牌面与终端

默认牌面使用 ASCII 单宽字符，跨 SSH、iTerm2、Windows Terminal 与常见 Linux 终端都能对齐：

```text
+--++--++--+
|m ||p ||s |
|1 ||3 ||9 |
+--++--++--+
```

`--cjk-tiles` 可切换为中文花色牌面，建议使用 Sarasa Mono、JetBrains Mono CN 或 Noto Sans Mono CJK 等等宽 CJK 字体。若出现错位，请回退默认 ASCII 模式。

## 会话与重连

客户端会把 `LoginResponse.session_token` 写入 `--token-file`，默认路径为 `~/.lsp/session.token`，文件权限为 `0600`。断线后会指数退避重连并携带该 token；服务端恢复成功时下发 `SnapshotNotify`，其中包含当前玩家手牌、弃牌堆与副露。

## 依赖边界

本命令使用 `github.com/rivo/tview` 与 `github.com/gdamore/tcell/v2` 仅做终端渲染，限定在 `cmd/cli/**` 内使用，不进入 `internal/` 的房间编排、协议处理或规则逻辑。
