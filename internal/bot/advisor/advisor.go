// Package advisor 定义可选 LLM 建议层接口；默认实现不引入外部依赖。
package advisor

import (
	"context"

	"racoo.cn/lsp/internal/bot"
)

// Advisor 基于公开 BotView 给出结构化动作建议。
type Advisor interface {
	Suggest(ctx context.Context, view bot.BotView, allowed []bot.ActionKind) (bot.Action, error)
}

// Mock 是测试与占位用 Advisor。
type Mock struct {
	Action bot.Action
	Err    error
}

// Suggest 返回预设动作。
func (m Mock) Suggest(_ context.Context, _ bot.BotView, _ []bot.ActionKind) (bot.Action, error) {
	return m.Action, m.Err
}
