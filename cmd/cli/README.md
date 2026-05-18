# lsp-cli 玩家终端客户端

`lsp-cli` 是纯终端玩家客户端，使用全屏大厅 + 纯中文四向对称牌桌。

大厅、房间预备、牌桌和结算都在 tcell 全屏内完成。主键位是 `←→` 选牌、`Enter` 提交、`Esc` 调出菜单、
`i` 看房间信息、`Tab` 看玩家详情、`Space` 在换三张时标记/取消，`m/p/s` 在定缺阶段选择万/筒/条。

## 启动

唯一启动说明见 [前端启动方式](../../docs/FRONTEND.md)。本文件只说明 `lsp-cli` 的交互和参数，不复制启动命令。

首次启动会询问昵称并把它连同会话令牌一起写到 `~/.lsp/config.toml`，
后续启动直接静默登录；服务端令牌失效或下发路由重定向时会自动清 token、跟随新地址重连。

## 命令行参数

```text
--config           本地配置文件路径 (默认 ~/.lsp/config.toml)
--name             覆盖配置中的昵称
--ws               覆盖配置中的服务器 WebSocket 地址
--origin           WebSocket Origin 头
--insecure-skip-verify   仅自签证书调试用,跳过 TLS 校验
--smoke-duration   非交互冒烟时长,例如 5s,CI 用
--version          打印版本后退出
```

## 大厅交互

```text
lsp · 大厅 · racoo                                      ● 23ms

        ┌ 快速开始 ┐   ┌ 创建房间 ┐   ┌ 加入房间码 ┐   ┌ 公开房间 ┐
        │ 自动补齐 │   │ 选择玩法 │   │ 好友邀请   │   │ 等候房   │
        │ Enter    │   │ Enter    │   │ Enter      │   │ Enter    │

←→/↑↓ 选择    Enter 确认    ? 帮助    q 退出
```

- **快速开始**：服务端有空座则进入，没有就开新公开房并入座。
- **创建房间**：先选玩法，再选公开性，最后确认房间名；私密房会显示房间码供分享。
- **加入房间码**：输入好友给你的房间码。
- **公开房间**：查看公开等候房并选择加入。

## 牌桌交互

牌桌框就是麻将桌面。自己永远在底部，下家在左，对家在上，上家在右；框内显示四家手牌、牌河和中央阶段提示。

按键约定：

| 按键 | 动作 |
| --- | --- |
| `←` `→` | 在手牌上左右移动光标 |
| `Enter` | 单选模式直接出牌；多选(换三张)模式提交已标记的 3 张 |
| `Space` | 多选模式标记/取消当前光标位置（换三张专用） |
| `m` / `p` / `s` | 定缺阶段选择万 / 筒 / 条 |
| `h` / `g` / `p` / `n` | Claim 浮窗中选择胡 / 杠 / 碰 / 过 |
| `Esc` | 调出 / 关闭局内菜单 |
| `i` | 调出 / 关闭房间信息浮窗（房号、规则、人数） |
| `Tab` | 调出 / 关闭玩家详情浮窗 |
| `q` | 返回大厅 / 离桌 |

claim 浮窗（碰/杠/胡/过）自带客户端倒计时，`←→` 切换按钮，`Enter` 确认，超时自动发送 `PassReq`。
服务端每个候选拥有完整 5 秒窗口，主动过后会接力下一位候选。

## 牌面与字体

牌面固定为纯中文：「一二三 / 万筒条 / 东南西北中发白」+ Unicode 框线。客户端不再提供 ASCII 或 emoji 主题切换。
建议使用支持 CJK 双宽字符的等宽字体，例如 Sarasa Mono、JetBrains Mono CN、Noto Sans Mono CJK。

## 终端尺寸

牌桌至少需要 `100×30` cell。低于这个尺寸时会提示“窗口太小，请调大终端”，玩家放大窗口后再试。

## 会话与重连

`config.toml` 同时持久化昵称、服务器与 SessionToken；
SessionToken 也会同步写到 `~/.lsp/session.token`，便于底层 `WSClient.login` 复用。
启动时 `SilentLogin` 自动用 token 续接：

- `LoginResp UNAUTHORIZED` 且本地有 token → 清 token 后重试一次（最多 3 次）。
- `LoginResp ROUTE_REDIRECT` 后等待同连接里的 `RouteRedirectNotify`；收到带完整 `ws_url` 的通知后，把 `ServerURL` 切到新地址并重连。正常同集群重连不应收到该通知。
- 牌桌阶段断线 → 居中浮窗显示"网络异常,重连中..."；30 秒未恢复出现"返回大厅"按钮。
- 局内菜单和网络离线浮窗返回大厅都会先发送 `LeaveRoomReq`，服务端确认后再回到 lobby。

## 调试命令栏

旧的命令栏 `CommandHandler` 只保留给 smoke/调试路径。玩家全流程的新动作都走全屏 TUI 与 `TableGateway`，
不再给命令栏新增平行动作，避免两套入口语义漂移。

## 依赖边界

本命令仅引入 `github.com/gdamore/tcell/v2` 做终端渲染、`github.com/pelletier/go-toml/v2`
做配置文件序列化，限定在 `cmd/cli/**` 范围内，不进入 `internal/` 房间编排、协议处理与规则逻辑。
旧的 `github.com/rivo/tview` 依赖已经移除。
