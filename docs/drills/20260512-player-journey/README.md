# 玩家旅程对照演练 · 20260512

## 本轮结论摘要（2026-05-12）

- **整体**：pass（cli 侧 11 段对照完成，A1/A2/A3/A10/A11/A12/A13/A14/A15/A16/A17/A18 全部修复或锁定回归；A4/A5/A6/A7/A8/A9/A19/A20 留作服务端 PR / UX 候选）。
- **测试基线**：`make verify-fast` 全绿（lint + go test + markdownlint + 大小写卫生 + secrets scan）。
- **新增条款级回归数量**：22 条 `TestPlayerJourney_*`（命名与 spec 条款一一对应），分布在 `cmd/cli/{state_apply,table_screen,seat_tiles,scene,dialog_settlement}_test.go`。
- **遗留候选**：A4（tenpai 投影协议字段）、A5（服务端超时 surrender 收口）、A6（服务端结算零和 panic）、A7（gate_remote DrawTile per-seat 隐私等价测试）、A8（杠四形态 cli 渲染）、A9（海底/杠上花 ScoringContext）、A19（多局之间「准备下一局」UX 提示）、A20（离桌弃局 confirm）。

本目录是按 [docs/spec/player-journey.md](../../spec/player-journey.md) v0.5 与 [docs/spec/architecture-gaps.md](../../spec/architecture-gaps.md) v0.1 做的"逐段对照演练"。每一段对应一份 markdown，记录：

- 该段每条 spec 条款是否被实际帧承载；
- 帧 dump 切片（来自 `LSP_FRAME_LOG`）与后端日志切片（来自 cmd/all stdout/stderr）；
- 若不符合，绑定到 `architecture-gaps.md` 中的某个 AID；
- 修复后回写"已修复 + 测试名"。

## 工具链

- 后端：`scripts/drills-dev.sh start`（起 `cmd/all`，日志重定向到 `tmp/drills/<ts>/backend.log`）。
- 普通前端启动统一见 [前端启动方式](../../FRONTEND.md)；本演练文档不复制启动命令。
- 演练调试才使用 `scripts/drills-dev.sh start --cli`；它会写 `tmp/drills/<ts>/frames.jsonl`。`tmp/drills` 是帧 dump 目录，不是前端启动目录。
- 帧 dump 只在 view 摘要变化时写一行，因此可直接 `jq` 过滤场景或字段：

  ```bash
  jq -c 'select(.scene=="table" and .waiting_action!="")' tmp/drills/latest/frames.jsonl
  ```

- 后端日志全 json：

  ```bash
  jq -c 'select(.module=="room" and .level=="error")' tmp/drills/latest/backend.log
  ```

## 演练分段（与 spec 1~10 节对齐）

| 段 | spec 节 | 子文件 | AID 锚点 | 状态 |
| --- | --- | --- | --- | --- |
| L 大厅 | spec §1 | [01-lobby.md](01-lobby.md) | A2、A12、A13 | pass |
| P 房间预备 | spec §2 | [02-room-prep.md](02-room-prep.md) | A1、A3 | pass |
| E 换三张 + Q 定缺 | spec §3 + §4 | [03-exchange-que.md](03-exchange-que.md) | A14、A15 | pass |
| D 摸打循环 | spec §5 | [04-draw-discard.md](04-draw-discard.md) | A16；A4/A5/A7/A8 待跟进 | pass（cli 部分） |
| C 抢答 + T 自摸窗口 | spec §6 + §7 | [05-claim-tsumo.md](05-claim-tsumo.md) | A17；A5/A8/A9 待跟进 | pass |
| S 结算 + R 再开一桌 | spec §8 + §9 | [06-settle-rematch.md](06-settle-rematch.md) | A10、A18；A6/A19/A20 待跟进 | pass（cli 部分） |
| N 断线重连 | spec §10 | [07-reconnect.md](07-reconnect.md) | A11；A5/A20 待跟进 | pass（cli 部分） |

每个分段文件使用 [TEMPLATE.md](TEMPLATE.md) 同一份骨架，避免格式漂移。

回归测试矩阵（条款 ↔ 测试名 ↔ AID 一一对应）见 [99-regression-matrix.md](99-regression-matrix.md)。

## 流程纪律

1. 演练只读 spec 与代码事实，不在 drill 文件里发明新规则。出现"实在不知道对错"的项，回 spec 提案修订，不就地裁决。
2. 每条结论必须给出"帧引用 + 日志引用 + 条款"三段证据；缺一段视为未结论。
3. 修复动作不在 drill 文件里贴 patch；只记录"目标修复责任 → 已绑定 AID → 测试名"。补丁本身走常规 PR + verify 流程。
4. drill 完成后在本 README 顶部加一行结论摘要（pass/fail/部分），不改写历史子文件。
