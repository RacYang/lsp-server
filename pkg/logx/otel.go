//go:build otel

package logx

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

func addOTelAttrs(ctx context.Context, rec *slog.Record, existing map[string]struct{}) {
	spanCtx := trace.SpanContextFromContext(ctx)
	if !spanCtx.IsValid() {
		return
	}
	addContextAttr(rec, existing, "trace_id", spanCtx.TraceID().String())
	addContextAttr(rec, existing, "span_id", spanCtx.SpanID().String())
}
