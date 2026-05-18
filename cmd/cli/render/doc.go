// Package render 是 CLI TUI 的纯绘制引擎。
//
// 本包只依赖 tcell 与 uniseg，不 import 任何游戏类型。
// 提供布局计算、语义色彩、统一组件库与四向对称牌桌绘制——由上层场景负责把 RoomView
// 翻译为 TableData 等纯数据 struct，再交给本包渲染。
//
// 设计原则：纯中文宽字符、硬编码 Unicode 框线、竖排手牌、框即牌桌。
//
// 职责边界：
//   - 绘制（Yes）：tcell 屏幕写入、框线、对话框、列表、牌桌、牌河
//   - 游戏逻辑（No）：RoomView 解释、光标状态机、按键分发、网络通信
package render
