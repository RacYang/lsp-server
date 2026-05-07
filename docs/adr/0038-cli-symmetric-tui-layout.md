---
title: lsp-cli 黑白四方对称 TUI 布局
status: accepted
date: 2026-05-07
---

# ADR-0038：lsp-cli 黑白四方对称 TUI 布局

## 状态

已采纳。

## 背景

旧牌桌界面按顶部状态栏、三家横排、自家手牌和底部键栏切分。该布局在等待开局时出现大量空白，也缺少局况、活跃玩家、出牌史与结算层次。玩家反馈其不像一张对称牌桌，也不符合桌面 16:9 / 16:10 终端的可用空间。

## 决策

- 牌桌 TUI 采用黑白美学，不引入颜色语义；用字符厚度、反白、密度块与徽章表达层级。
- 最小尺寸为 `100x30`，推荐尺寸为 `140x40`；低于最小尺寸时拒绝进入牌桌并提示放大。
- 布局拓扑固定为四方对称：北/南横排，西/东单行紧凑，中央桌面位于几何中心。
- `RunTableScreen` 持有 `lastTier`，`CalcLayout` 用三档断点和 5 cell 升档滞回减少 resize 抖动。
- 渲染阶段统一由 `DerivePhase` 派生，中央桌面、按键栏和帮助浮窗消费同一阶段。
- 玩家面板使用三档边框语言：默认薄线、当前活跃座粗线、自己双线。
- HUD 借鉴 TUI 标杆：
  - Cogmind：状态徽章、残牌进度条与关键事件反白。
  - Brogue / lazygit：边框即焦点。
  - btop：密度块与 sparkline。
  - vim：反白 modeline 表示模式。
  - k9s：面包屑式上下文。
  - Caves of Qud / chess TUI：最近事件 / 出牌史。
  - glow：结算页采用 markdown 风格层次。
- 牌面主题支持 `unicode`、`ascii` 与 `emoji`。`emoji` 使用 Unicode Mahjong Tiles，仅在用户主动选择时启用。

## 后果

- 客户端更接近真实牌桌方位，等待、出牌、鸣牌和结算都有清晰阶段反馈。
- 80x24 不再作为目标尺寸；桌面终端需至少 100x30。
- 旧 `layout.Wide` 与旧四区布局直接 cut over，不保留 `LSP_CLI_LEGACY_TABLE`。
- emoji 主题可能受终端字体影响，需要保留 unicode/ascii 作为稳定默认与降级路径。
