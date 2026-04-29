package logx

import (
	"context"
	"testing"
)

func TestWithTraceID(t *testing.T) {
	ctx := context.Background()
	ctx = WithTraceID(ctx, "tid-1")
	if got := TraceIDFromContext(ctx); got != "tid-1" {
		t.Fatalf("TraceIDFromContext = %q, want tid-1", got)
	}
}

func TestWithUserID(t *testing.T) {
	ctx := WithUserID(context.Background(), "u-9")
	if got := UserIDFromContext(ctx); got != "u-9" {
		t.Fatalf("UserIDFromContext = %q, want u-9", got)
	}
}

func TestWithRoomID(t *testing.T) {
	ctx := WithRoomID(context.Background(), "r-3")
	if got := RoomIDFromContext(ctx); got != "r-3" {
		t.Fatalf("RoomIDFromContext = %q, want r-3", got)
	}
}

func TestWithFields(t *testing.T) {
	ctx := WithFields(context.Background(), "rule_id", "sichuan_xzdd")
	fields := FieldsFromContext(ctx)
	if len(fields) != 1 || fields[0].Key != "rule_id" || fields[0].Value.String() != "sichuan_xzdd" {
		t.Fatalf("fields = %#v", fields)
	}
}
