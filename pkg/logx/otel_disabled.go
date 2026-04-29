//go:build !otel

package logx

import (
	"context"
	"log/slog"
)

func addOTelAttrs(_ context.Context, _ *slog.Record, _ map[string]struct{}) {}
