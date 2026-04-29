package logx

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLoggerInfoJSON(t *testing.T) {
	var buf bytes.Buffer
	log := New(&buf, LevelInfo)
	ctx := context.Background()
	ctx = WithTraceID(ctx, "t1")
	ctx = WithUserID(ctx, "u1")
	ctx = WithRoomID(ctx, "r1")
	log.Info(ctx, "测试信息写入", "extra", 1)
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
	if _, ok := m["source"]; !ok {
		t.Fatalf("source missing: %#v", m)
	}
}

func TestLoggerLevels(t *testing.T) {
	var buf bytes.Buffer
	log := New(&buf, LevelDebug)
	ctx := context.Background()
	log.Debug(ctx, "调试")
	log.Warn(ctx, "警告")
	log.Error(ctx, "错误")
	s := buf.String()
	if !strings.Contains(s, "调试") || !strings.Contains(s, "警告") || !strings.Contains(s, "错误") {
		t.Fatalf("unexpected output: %s", s)
	}
}

func TestPackageLevelInfo(t *testing.T) {
	var buf bytes.Buffer
	// 替换默认 logger 以便断言（仅测试）。
	old := Default()
	SetDefault(New(&buf, LevelInfo))
	t.Cleanup(func() { SetDefault(old) })
	ctx := context.Background()
	Info(ctx, "包级信息")
	if !strings.Contains(buf.String(), "包级信息") {
		t.Fatalf("expected 包级信息 in %s", buf.String())
	}
}

func TestLoggerWithObserver(t *testing.T) {
	obs, log := NewObserver()
	ctx := WithTraceID(context.Background(), "tid")
	ctx = WithUserID(ctx, "uid")
	ctx = WithRoomID(ctx, "rid")
	log.With("op", "join_room").Info(ctx, "玩家进入房间", "rule_id", "sichuan_xzdd")
	entries := obs.Drain()
	if len(entries) != 1 {
		t.Fatalf("entries = %d", len(entries))
	}
	got := entries[0].Attrs
	if got["trace_id"] != "tid" || got["user_id"] != "uid" || got["room_id"] != "rid" || got["op"] != "join_room" {
		t.Fatalf("attrs: %#v", got)
	}
}

func TestLoggerRedactsSensitiveKeys(t *testing.T) {
	var buf bytes.Buffer
	log := NewWithOptions(&buf, LevelInfo, Options{
		Format:        "json",
		IncludeSource: true,
		RedactKeys:    []string{"token"},
	})
	log.Info(context.Background(), "写入敏感字段", "token", "secret-value")
	var m map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &m); err != nil {
		t.Fatal(err)
	}
	if m["token"] != redactedValue {
		t.Fatalf("token = %#v", m["token"])
	}
}

func TestSamplingKeepsErrors(t *testing.T) {
	var buf bytes.Buffer
	log := NewWithOptions(&buf, LevelDebug, Options{
		Format:        "json",
		IncludeSource: true,
		Sampling: SamplingConfig{
			Enabled:           true,
			Initial:           0,
			Thereafter:        100,
			Tick:              time.Minute,
			ErrorNeverSampled: true,
		},
	})
	log.Info(context.Background(), "普通信息")
	log.Error(context.Background(), "错误信息")
	if !strings.Contains(buf.String(), "错误信息") {
		t.Fatalf("expected error log, got %s", buf.String())
	}
}

func TestAtomicLevel(t *testing.T) {
	var buf bytes.Buffer
	level := NewAtomicLevel(LevelError)
	log := NewWithOptions(&buf, LevelDebug, Options{Format: "json", IncludeSource: true, AtomicLevel: level})
	log.Info(context.Background(), "普通信息")
	level.SetLevel(LevelInfo)
	log.Info(context.Background(), "恢复信息")
	if strings.Contains(buf.String(), "普通信息") || !strings.Contains(buf.String(), "恢复信息") {
		t.Fatalf("unexpected output: %s", buf.String())
	}
}

func TestObserverConcurrent(t *testing.T) {
	obs, log := NewObserver()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			log.Info(context.Background(), "并发日志")
		}()
	}
	wg.Wait()
	if got := len(obs.Drain()); got != 8 {
		t.Fatalf("entries = %d", got)
	}
}
