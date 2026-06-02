// Package config 负责加载运行时配置（viper 读取 YAML）。
//
// 职责：将 YAML 文件解析为 Config 快照；定义进程级运行时参数类型。
// 禁止在本包内持有配置单例；调用方自行决定生命周期。
package config
