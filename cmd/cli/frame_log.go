package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// FrameLogger 是联调显微镜：把每一帧 SceneRouter 渲染前的关键状态以 JSONL 形式
// 落盘。只在 LSP_FRAME_LOG 环境变量非空时启用，且只在状态摘要发生变化时写入，
// 用来对照玩家旅程 spec 条款 → 实际帧 → 后端日志切片。
//
// 字段集合刻意精简：与 ADR-0044 五类事实对齐的子集（RoomLifecycle / RoundProgress
// / SeatRoster 关键字段 + 必要的 UXTransient 文案锚点），避免把整个 RoomView 全量
// 序列化造成噪音。
type FrameLogger struct {
	mu      sync.Mutex
	writer  *os.File
	encoder *json.Encoder
	lastSum string
	seq     uint64
}

// NewFrameLoggerFromEnv 读取 LSP_FRAME_LOG 环境变量。空值返回 nil，调用方需做 nil 安全。
// 路径所在目录会被自动创建。打开失败仅打印一行 stderr，不阻塞 cli 启动。
func NewFrameLoggerFromEnv() *FrameLogger {
	path := os.Getenv("LSP_FRAME_LOG")
	if path == "" {
		return nil
	}
	cleanPath := filepath.Clean(path)
	if dir := filepath.Dir(cleanPath); dir != "" && dir != "." {
		_ = os.MkdirAll(dir, 0o750) //nolint:gosec // LSP_FRAME_LOG 是显式开发态环境变量，开发者控制路径
	}
	f, err := os.OpenFile(cleanPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600) //nolint:gosec // 同上
	if err != nil {
		fmt.Fprintf(os.Stderr, "frame_log: 打开 %s 失败: %v\n", path, err)
		return nil
	}
	return &FrameLogger{writer: f, encoder: json.NewEncoder(f)}
}

// Capture 写入一帧；仅当语义摘要与上一帧不同才落盘，避免每 50ms tick 都打一行。
func (l *FrameLogger) Capture(sceneID SceneID, view RoomView, now time.Time) {
	if l == nil {
		return
	}
	rec := buildFrameRecord(sceneID, view, now)
	sum := rec.digest()
	l.mu.Lock()
	defer l.mu.Unlock()
	if sum == l.lastSum {
		return
	}
	l.seq++
	rec.Seq = l.seq
	if err := l.encoder.Encode(rec); err != nil {
		fmt.Fprintf(os.Stderr, "frame_log: 写入失败: %v\n", err)
		return
	}
	l.lastSum = sum
}

// Close 刷盘并关闭句柄。nil 安全。
func (l *FrameLogger) Close() error {
	if l == nil || l.writer == nil {
		return nil
	}
	return l.writer.Close()
}

type frameRecord struct {
	Seq              uint64              `json:"seq"`
	TS               string              `json:"ts"`
	Scene            string              `json:"scene"`
	UserID           string              `json:"user_id,omitempty"`
	RoomID           string              `json:"room_id,omitempty"`
	SeatIndex        int32               `json:"seat_index"`
	RoomState        string              `json:"room_state,omitempty"`
	WaitingAction    string              `json:"waiting_action,omitempty"`
	RoundPhase       string              `json:"round_phase,omitempty"`
	ActingSeat       int32               `json:"acting_seat"`
	ActingSeats      []int32             `json:"acting_seats,omitempty"`
	AvailableActions []string            `json:"available_actions,omitempty"`
	ClaimCandidates  map[string][]string `json:"claim_candidates,omitempty"`
	WallRemaining    int32               `json:"wall_remaining"`
	DeadlineUnixMS   int64               `json:"deadline_unix_ms,omitempty"`
	LastStep         int64               `json:"last_step,omitempty"`
	SnapshotStep     int64               `json:"snapshot_step,omitempty"`
	Seats            []frameSeat         `json:"seats,omitempty"`
	UXNotice         string              `json:"ux_notice,omitempty"`
	LastAction       string              `json:"last_action,omitempty"`
	Connected        bool                `json:"connected"`
	Reconnecting     bool                `json:"reconnecting,omitempty"`
}

type frameSeat struct {
	Seat        int    `json:"seat"`
	UserID      string `json:"user_id,omitempty"`
	Nickname    string `json:"nickname,omitempty"`
	Status      string `json:"status,omitempty"`
	Online      bool   `json:"online"`
	IsBot       bool   `json:"bot,omitempty"`
	Ready       bool   `json:"ready,omitempty"`
	Surrendered bool   `json:"surrendered,omitempty"`
	AutoPlay    bool   `json:"auto_play,omitempty"` // 仍上报，用于 [G12] 回归断言
	Hued        bool   `json:"hued,omitempty"`
	HandCnt     int    `json:"hand_cnt"`
	TotalScore  int32  `json:"total_score"`
}

func buildFrameRecord(sceneID SceneID, view RoomView, now time.Time) frameRecord {
	rec := frameRecord{
		TS:               now.UTC().Format(time.RFC3339Nano),
		Scene:            string(sceneID),
		UserID:           view.UserID,
		RoomID:           view.RoomID,
		SeatIndex:        view.SeatIndex,
		RoomState:        view.RoomState,
		WaitingAction:    view.WaitingAction,
		RoundPhase:       view.RoundPhase.String(),
		ActingSeat:       view.ActingSeat,
		ActingSeats:      append([]int32(nil), view.ActingSeats...),
		AvailableActions: append([]string(nil), view.AvailableActions...),
		WallRemaining:    view.WallRemaining,
		DeadlineUnixMS:   view.DeadlineUnixMS,
		LastStep:         view.LastStep,
		SnapshotStep:     view.SnapshotStep,
		UXNotice:         view.UXNotice,
		Connected:        view.Connected,
		Reconnecting:     view.Reconnecting,
	}
	if len(view.ClaimCandidates) > 0 {
		rec.ClaimCandidates = make(map[string][]string, len(view.ClaimCandidates))
		for seat, actions := range view.ClaimCandidates {
			rec.ClaimCandidates[fmt.Sprintf("%d", seat)] = append([]string(nil), actions...)
		}
	}
	if view.LastAction != nil {
		rec.LastAction = fmt.Sprintf("seat=%d action=%s tile=%s step=%d", view.LastAction.GetActorSeat(), view.LastAction.GetAction(), view.LastAction.GetTile(), view.LastAction.GetStep())
	}
	seats := make([]frameSeat, 0, 4)
	for i, p := range view.Players {
		if p.UserID == "" && !p.IsBot && p.Nickname == "" {
			continue
		}
		seats = append(seats, frameSeat{
			Seat:        i,
			UserID:      p.UserID,
			Nickname:    p.Nickname,
			Status:      p.Status,
			Online:      p.Online,
			IsBot:       p.IsBot,
			Ready:       p.Ready,
			Surrendered: p.Surrendered,
			AutoPlay:    p.AutoPlay,
			Hued:        p.Hued,
			HandCnt:     p.HandCnt,
			TotalScore:  p.TotalScore,
		})
	}
	rec.Seats = seats
	return rec
}

func (r frameRecord) digest() string {
	clone := r
	clone.TS = ""
	clone.Seq = 0
	if len(clone.ClaimCandidates) > 0 {
		keys := make([]string, 0, len(clone.ClaimCandidates))
		for k := range clone.ClaimCandidates {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		ordered := make(map[string][]string, len(keys))
		for _, k := range keys {
			ordered[k] = clone.ClaimCandidates[k]
		}
		clone.ClaimCandidates = ordered
	}
	payload, err := json.Marshal(clone)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}
