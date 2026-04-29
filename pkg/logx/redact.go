package logx

import (
	"context"
	"log/slog"
)

const redactedValue = "***"

func newRedactHandler(next slog.Handler, keys []string) slog.Handler {
	blocked := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		blocked[key] = struct{}{}
	}
	return handlerWrapper{
		next: next,
		fn: func(_ctx context.Context, rec slog.Record) slog.Record {
			var attrs []slog.Attr
			rec.Attrs(func(attr slog.Attr) bool {
				attrs = append(attrs, redactAttr(attr, blocked))
				return true
			})
			out := slog.NewRecord(rec.Time, rec.Level, rec.Message, rec.PC)
			out.AddAttrs(attrs...)
			return out
		},
	}
}

func redactAttr(attr slog.Attr, blocked map[string]struct{}) slog.Attr {
	if _, ok := blocked[attr.Key]; ok {
		return slog.String(attr.Key, redactedValue)
	}
	if attr.Value.Kind() != slog.KindGroup {
		return attr
	}
	children := attr.Value.Group()
	for i := range children {
		children[i] = redactAttr(children[i], blocked)
	}
	return slog.Group(attr.Key, attrsToAny(children)...)
}

func attrsToAny(attrs []slog.Attr) []any {
	out := make([]any, 0, len(attrs))
	for _, attr := range attrs {
		out = append(out, attr)
	}
	return out
}
