# <段名> · TEMPLATE

> 复制本文件为 `0X-<段名>.md` 后填写。删除本说明段。

## 0. 上下文

- spec 节：§<X>
- drill 目录：`tmp/drills/<ts>/`
- 后端配置：`configs/dev.yaml`
- 时间窗：`<开始>` → `<结束>`
- 涉及 AID：`A<n>`、`A<m>`（与 [docs/spec/architecture-gaps.md](../../spec/architecture-gaps.md) 对齐）

## 1. 条款对照表

| 条款 | 等级 | 预期帧/事实 | 实际帧切片 | 后端日志切片 | 结论 | AID/测试名 |
| --- | --- | --- | --- | --- | --- | --- |
| `[X1.1]` | MUST | <一行预期> | `frames.jsonl: <line>` | `backend.log: <line>` | pass / fail / 部分 | `A<n>` / `TestPlayerJourney_X1_1_*` |

> 帧切片建议直接贴 `jq -c` 后的一行 JSON；不要超过 200 字符；超过则截断并贴文件偏移。
> 后端日志切片同理。
> "结论"必须三选一，不允许"待确认"——待确认就别合入演练。

## 2. 关键发现

- 现象：<一行描述玩家看到的不对劲>
- 根因猜测：<协议 / 服务端 / 网关 / cli reducer / cli scene>
- 锚点条款：`[Xn.m]`
- AID：`A<n>`

## 3. 修复跟踪

- [ ] AID `A<n>` 已生成回归测试 `TestPlayerJourney_..._...`
- [ ] 修复 PR：<链接或 commit>
- [ ] 演练复跑：`tmp/drills/<新 ts>/` 已断言通过

## 4. 留底素材

- frames.jsonl 关键片段路径：`<相对路径>`
- backend.log 关键片段路径：`<相对路径>`
- 屏幕快照（如有）：`<相对路径>`
