// Package clock 提供可注入时间源，避免房间定时器测试依赖真实 sleep。
//
// 职责：抽象 Now() 与 AfterFunc() 两个原语；生产路径注入 realClock，单元测试注入 Fake。
// 禁止在本包内引入任何业务逻辑。
package clock
