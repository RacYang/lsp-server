package logx

import (
	"context"
	"log/slog"
	"regexp"
)

// FieldSchema 定义日志字段名治理规则。
type FieldSchema struct {
	Pattern      string
	MaxLength    int
	CoreKeys     []string
	ForbiddenKey []string
	PIIKeys      []string
}

func newSchemaHandler(next slog.Handler, schema FieldSchema) slog.Handler {
	var pat *regexp.Regexp
	if schema.Pattern != "" {
		pat = regexp.MustCompile(schema.Pattern)
	}
	forbidden := stringSet(schema.ForbiddenKey)
	pii := stringSet(schema.PIIKeys)
	return handlerWrapper{
		next: next,
		fn: func(_ctx context.Context, rec slog.Record) slog.Record {
			var bad []string
			rec.Attrs(func(attr slog.Attr) bool {
				collectSchemaViolations(attr, pat, schema.MaxLength, forbidden, pii, &bad)
				return true
			})
			if len(bad) > 0 {
				rec.AddAttrs(slog.Any("_schema_violation", bad))
			}
			return rec
		},
	}
}

func collectSchemaViolations(
	attr slog.Attr,
	pat *regexp.Regexp,
	maxLen int,
	forbidden map[string]struct{},
	pii map[string]struct{},
	bad *[]string,
) {
	if attr.Key == "" {
		return
	}
	if _, ok := forbidden[attr.Key]; ok {
		*bad = append(*bad, attr.Key)
	}
	if _, ok := pii[attr.Key]; ok {
		*bad = append(*bad, attr.Key)
	}
	if pat != nil && !pat.MatchString(attr.Key) {
		*bad = append(*bad, attr.Key)
	}
	if maxLen > 0 && len(attr.Key) > maxLen {
		*bad = append(*bad, attr.Key)
	}
	if attr.Value.Kind() != slog.KindGroup {
		return
	}
	for _, child := range attr.Value.Group() {
		collectSchemaViolations(child, pat, maxLen, forbidden, pii, bad)
	}
}

func stringSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}
