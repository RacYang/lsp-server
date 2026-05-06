//go:build llm_openai

package advisor

import (
	"context"
	"fmt"

	"racoo.cn/lsp/internal/bot"
)

// OpenAICompatConfig 定义 OpenAI 兼容 Provider 的最小配置。
//
// 该配置刻意保持为纯字符串字段，避免第一版把任何外部 SDK 类型暴露到默认代码路径。
type OpenAICompatConfig struct {
	BaseURL string
	APIKey  string
	Model   string
}

// OpenAICompat 是预留的 OpenAI/DeepSeek 兼容适配器占位。
//
// 真正请求外部模型的实现应继续放在 llm_openai build tag 下，并确保默认构建不产生网络依赖。
type OpenAICompat struct {
	Config OpenAICompatConfig
}

// Suggest 当前仅返回未实现，真实 Provider 后续在 llm_openai build tag 下补齐。
func (o OpenAICompat) Suggest(_ context.Context, _ bot.BotView, _ []bot.ActionKind) (bot.Action, error) {
	return bot.Action{}, fmt.Errorf("openai compatible advisor not implemented: %s", o.Config.Model)
}
