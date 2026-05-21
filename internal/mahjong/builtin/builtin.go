// Package builtin 统一注册生产可用的麻将规则包。
package builtin

import (
	_ "racoo.cn/lsp/internal/mahjong/guobiao/jingji"       // 注册内置国标竞技规则。
	_ "racoo.cn/lsp/internal/mahjong/sichuan/xuezhandaodi" // 注册当前内置四川血战规则。
)
