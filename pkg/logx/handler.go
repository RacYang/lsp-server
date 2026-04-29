package logx

import (
	"context"
	"log/slog"
)

type handlerWrapper struct {
	next slog.Handler
	fn   func(context.Context, slog.Record) slog.Record
}

func (h handlerWrapper) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h handlerWrapper) Handle(ctx context.Context, rec slog.Record) error {
	return h.next.Handle(ctx, h.fn(ctx, rec.Clone()))
}

func (h handlerWrapper) WithAttrs(attrs []slog.Attr) slog.Handler {
	return handlerWrapper{next: h.next.WithAttrs(attrs), fn: h.fn}
}

func (h handlerWrapper) WithGroup(name string) slog.Handler {
	return handlerWrapper{next: h.next.WithGroup(name), fn: h.fn}
}

func newContextHandler(next slog.Handler) slog.Handler {
	return handlerWrapper{
		next: next,
		fn: func(ctx context.Context, rec slog.Record) slog.Record {
			existing := recordKeys(rec)
			addContextAttr(&rec, existing, "trace_id", TraceIDFromContext(ctx))
			addContextAttr(&rec, existing, "user_id", UserIDFromContext(ctx))
			addContextAttr(&rec, existing, "room_id", RoomIDFromContext(ctx))
			for _, attr := range FieldsFromContext(ctx) {
				addContextAttr(&rec, existing, attr.Key, attr.Value.Any())
			}
			addOTelAttrs(ctx, &rec, existing)
			return rec
		},
	}
}

func addContextAttr(rec *slog.Record, existing map[string]struct{}, key string, value any) {
	if key == "" || value == nil {
		return
	}
	if s, ok := value.(string); ok && s == "" {
		return
	}
	if _, ok := existing[key]; ok {
		return
	}
	rec.AddAttrs(slog.Any(key, value))
	existing[key] = struct{}{}
}

func recordKeys(rec slog.Record) map[string]struct{} {
	keys := make(map[string]struct{})
	rec.Attrs(func(attr slog.Attr) bool {
		keys[attr.Key] = struct{}{}
		return true
	})
	return keys
}
