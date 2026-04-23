package logx

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestLoggerInfoJSON(t *testing.T) {
	var buf bytes.Buffer
	log := New(&buf, LevelInfo)
	ctx := context.Background()
	ctx = WithTraceID(ctx, "t1")
	ctx = WithUserID(ctx, "u1")
	ctx = WithRoomID(ctx, "r1")
	log.Info(ctx, "测试信息写入", "trace_id", TraceIDFromContext(ctx), "user_id", UserIDFromContext(ctx), "room_id", RoomIDFromContext(ctx), "extra", 1)
	line := strings.TrimSpace(buf.String())
	var m map[string]any
	if err := json.Unmarshal([]byte(line), &m); err != nil {
		t.Fatalf("json: %v, line=%s", err, line)
	}
	if m["msg"] != "测试信息写入" {
		t.Fatalf("msg = %v", m["msg"])
	}
	if m["trace_id"] != "t1" || m["user_id"] != "u1" || m["room_id"] != "r1" {
		t.Fatalf("fields: %#v", m)
	}
}

func TestLoggerLevels(t *testing.T) {
	var buf bytes.Buffer
	log := New(&buf, LevelDebug)
	ctx := context.Background()
	log.Debug(ctx, "调试", "trace_id", "", "user_id", "", "room_id", "")
	log.Warn(ctx, "警告", "trace_id", "", "user_id", "", "room_id", "")
	log.Error(ctx, "错误", "trace_id", "", "user_id", "", "room_id", "")
	s := buf.String()
	if !strings.Contains(s, "调试") || !strings.Contains(s, "警告") || !strings.Contains(s, "错误") {
		t.Fatalf("unexpected output: %s", s)
	}
}

func TestPackageLevelInfo(t *testing.T) {
	var buf bytes.Buffer
	// 替换默认 logger 以便断言（仅测试）。
	old := defaultLogger
	defaultLogger = New(&buf, LevelInfo)
	t.Cleanup(func() { defaultLogger = old })
	ctx := context.Background()
	Info(ctx, "包级信息", "trace_id", "x", "user_id", "y", "room_id", "z")
	if !strings.Contains(buf.String(), "包级信息") {
		t.Fatalf("expected 包级信息 in %s", buf.String())
	}
}
