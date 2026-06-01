package actor

import (
	"context"
	"time"

	"racoo.cn/lsp/internal/clock"
	"racoo.cn/lsp/internal/service/room/engine"
)

// Config 持有 Actor 构造所需的全部依赖与策略配置。
type Config struct {
	Engine                *engine.Engine
	Clock                 clock.Clock
	Capacity              int
	OnExit                func(roomID string)
	OnAutoTimeout         func(ctx context.Context, roomID string, notifications []engine.Notification)
	OnAfterCmd            func(roomID string)
	AllowLeaveDuringPlay  bool
	OfflineSurrenderAfter time.Duration
}
