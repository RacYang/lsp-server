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
