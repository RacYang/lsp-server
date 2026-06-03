// Package builtin 统一注册生产可用的麻将规则包。
//
// 职责：通过 blank import 触发 guobiao/jingji 与 sichuan/xuezhandaodi 的 init 注册。
// 业务层 import 本包即可获得全部内置规则，无须分别 import 各规则子目录。
package builtin
