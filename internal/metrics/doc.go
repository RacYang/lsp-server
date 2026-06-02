// Package metrics 集中定义跨层业务指标，避免各层重复注册或出现异名指标。
//
// 职责：Prometheus 指标变量注册；提供 ObserveStorage 等快捷记录函数。
// 禁止在本包内引入业务逻辑；指标命名须符合 lsp_ 前缀与 SSOT 后缀约束。
package metrics
