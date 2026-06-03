// Package rules 定义可插拔麻将规则接口与注册表；具体变体放在子目录中实现。
//
// 职责：Rule 接口、CapabilitySet 能力组合、全局注册表（Register/Lookup）与结算数据类型。
// 禁止引入传输、会话、存储或 app 层包。
package rules
