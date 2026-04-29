// lsp-cli 是面向玩家的终端客户端，负责把 client.v1 事件转换为本地牌桌视图。
package main

import (
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
)

const (
	stageIdle       = "idle"
	stageExchange   = "exchange"
	stageQueMen     = "quemen"
	stageDiscard    = "discard"
	stageClaim      = "claim"
	stageSettlement = "settlement"
)

const (
	phaseLogin = "login"
	phaseLobby = "lobby"
	phaseTable = "table"
)

// AppState 以互斥锁保护 TUI 渲染所需的牌桌快照。
type AppState struct {
	mu   sync.RWMutex
	view RoomView
}

// RoomView 是渲染层唯一读取的数据结构。
type RoomView struct {
	UserID       string
	Nickname     string
	ServerURL    string
	Phase        string
	RoomID       string
	SessionToken string
	SeatIndex    int32
	Stage        string

	DealerSeat      int32
	ActingSeat      int32
	PendingTile     string
	AvailableAction []string
	ClaimCandidates map[int32][]string
	QueBySeat       [4]int32
	Players         [4]PlayerView

	LastSettlement *clientv1.SettlementNotify
	RoomList       []*clientv1.RoomMeta
	NextRoomPage   string
	Log            []LogEntry
	LastError      string
	RTTms          int64
	Reconnecting   bool
	Connected      bool
	UpdatedAt      time.Time
}

// PlayerView 保存单个座位的客户端可见视图。
type PlayerView struct {
	UserID   string
	Nickname string
	Ready    bool
	Hand     []string
	HandCnt  int
	Melds    []string
	Discards []string
	Hued     bool
}

// LogEntry 是事件流中的一行可读消息。
type LogEntry struct {
	At   time.Time
	Text string
}

func NewAppState(name string) *AppState {
	st := &AppState{}
	st.view.Nickname = name
	st.view.SeatIndex = -1
	st.view.ActingSeat = -1
	st.view.Stage = stageIdle
	st.view.Phase = phaseLogin
	st.view.ClaimCandidates = make(map[int32][]string)
	for i := range st.view.QueBySeat {
		st.view.QueBySeat[i] = -1
	}
	st.addLogLocked("客户端已启动")
	return st
}

func (s *AppState) Snapshot() RoomView {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneRoomView(s.view)
}

func (s *AppState) Mutate(fn func(*RoomView)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(&s.view)
	s.view.UpdatedAt = time.Now()
}

func (s *AppState) AddLog(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.addLogLocked(text)
}

func (s *AppState) addLogLocked(text string) {
	s.view.Log = append(s.view.Log, LogEntry{At: time.Now(), Text: text})
	if len(s.view.Log) > 100 {
		s.view.Log = append([]LogEntry(nil), s.view.Log[len(s.view.Log)-100:]...)
	}
	s.view.UpdatedAt = time.Now()
}

func cloneRoomView(in RoomView) RoomView {
	out := in
	out.AvailableAction = append([]string(nil), in.AvailableAction...)
	out.ClaimCandidates = make(map[int32][]string, len(in.ClaimCandidates))
	for seat, actions := range in.ClaimCandidates {
		out.ClaimCandidates[seat] = append([]string(nil), actions...)
	}
	for i := range in.Players {
		out.Players[i] = clonePlayerView(in.Players[i])
	}
	out.RoomList = cloneRoomMetas(in.RoomList)
	out.Log = append([]LogEntry(nil), in.Log...)
	return out
}

func clonePlayerView(in PlayerView) PlayerView {
	out := in
	out.Hand = append([]string(nil), in.Hand...)
	out.Melds = append([]string(nil), in.Melds...)
	out.Discards = append([]string(nil), in.Discards...)
	return out
}

func cloneRoomMetas(in []*clientv1.RoomMeta) []*clientv1.RoomMeta {
	out := make([]*clientv1.RoomMeta, 0, len(in))
	for _, room := range in {
		if room == nil {
			continue
		}
		out = append(out, proto.Clone(room).(*clientv1.RoomMeta))
	}
	return out
}
