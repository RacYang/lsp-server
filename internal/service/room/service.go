// Package room 提供房间应用服务：加入、准备与开局编排。
package room

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"racoo.cn/lsp/internal/clock"
	domainroom "racoo.cn/lsp/internal/domain/room"
)

// Service 编排房间命令；每房间在内部通过 roomActor 单协程串行化变更。
type Service struct {
	lobby                *RoomRegistry
	mu                   sync.Mutex
	actors               map[string]*roomActor
	engine               *Engine
	clock                clock.Clock
	tmo                  TimeoutConfig
	maxHands             int32
	mailboxCapacity      int
	onAuto               func(context.Context, string, []Notification)
	allowLeaveDuringPlay bool
	// onAfterCmd 在 actor 处理完一条命令后触发，BotSupervisor 借此接力推动机器人座位。
	// 这条回调必须自身保持非阻塞（典型实现是丢给独立 goroutine 处理），否则会拖慢 actor 主循环。
	onAfterCmd func(roomID string)
}

// TimeoutConfig 定义各等待态的服务端托管时长。
type TimeoutConfig struct {
	OpeningDefault  time.Duration
	OpeningByAction map[string]time.Duration
	ClaimWindow     time.Duration
	TsumoWindow     time.Duration
	Discard         time.Duration
	SurrenderAction time.Duration
}

// DefaultTimeoutConfig 返回 Phase 5 定时器默认值。
func DefaultTimeoutConfig() TimeoutConfig {
	return TimeoutConfig{
		OpeningDefault:  15 * time.Second,
		ClaimWindow:     5 * time.Second,
		TsumoWindow:     15 * time.Second,
		Discard:         15 * time.Second,
		SurrenderAction: time.Second,
	}
}

func (cfg TimeoutConfig) withDefaults() TimeoutConfig {
	def := DefaultTimeoutConfig()
	hasOpeningActionOverrides := cfg.OpeningByAction != nil
	if cfg.OpeningDefault <= 0 {
		cfg.OpeningDefault = def.OpeningDefault
	}
	openingByAction := make(map[string]time.Duration)
	if !hasOpeningActionOverrides && cfg.OpeningDefault == def.OpeningDefault {
		for action, dur := range def.OpeningByAction {
			openingByAction[action] = dur
		}
	} else {
		for action, dur := range cfg.OpeningByAction {
			if action != "" && dur > 0 {
				openingByAction[action] = dur
			}
		}
	}
	cfg.OpeningByAction = openingByAction
	if cfg.ClaimWindow <= 0 {
		cfg.ClaimWindow = def.ClaimWindow
	}
	if cfg.TsumoWindow <= 0 {
		cfg.TsumoWindow = def.TsumoWindow
	}
	if cfg.Discard <= 0 {
		cfg.Discard = def.Discard
	}
	if cfg.SurrenderAction <= 0 {
		cfg.SurrenderAction = def.SurrenderAction
	}
	return cfg
}

// NewService 创建房间服务（广播由 handler 在写完应答帧后调用 Hub 完成）。
func NewService(l *RoomRegistry) *Service {
	return NewServiceWithRule(l, "")
}

// NewServiceWithRule 使用指定规则装配房间服务；ruleID 为空时回退默认四川血战规则。
func NewServiceWithRule(l *RoomRegistry, ruleID string) *Service {
	return &Service{
		lobby:                l,
		actors:               make(map[string]*roomActor),
		engine:               NewEngine(ruleID),
		clock:                clock.NewReal(),
		tmo:                  DefaultTimeoutConfig(),
		maxHands:             1,
		mailboxCapacity:      defaultMailboxCapacity,
		allowLeaveDuringPlay: true,
	}
}

// SetClock 注入时间源；主要供测试使用。
func (s *Service) SetClock(c clock.Clock) {
	if s == nil || c == nil {
		return
	}
	s.clock = c
	if s.engine != nil {
		s.engine.SetClock(c)
	}
}

// SetTimeoutConfig 覆盖房间托管时长。
func (s *Service) SetTimeoutConfig(cfg TimeoutConfig) {
	if s == nil {
		return
	}
	if s.engine != nil {
		s.engine.SetTimeoutConfig(cfg)
	}
	s.tmo = cfg.withDefaults()
}

// SetMailboxCapacity 覆盖新建房间 actor 的 mailbox 容量；非正值回退默认值。
func (s *Service) SetMailboxCapacity(capacity int) {
	if s == nil {
		return
	}
	if capacity <= 0 {
		capacity = defaultMailboxCapacity
	}
	s.mailboxCapacity = capacity
}

// SetMaxHands 设置同房最多连开局数；默认 1 局保持既有关闭行为。
func (s *Service) SetMaxHands(maxHands int32) {
	if s == nil {
		return
	}
	if maxHands <= 0 {
		maxHands = 1
	}
	s.maxHands = maxHands
}

// SetAllowLeaveDuringPlay 控制 playing 中 Leave 是否转换为 surrender；关闭时回退为拒绝离房。
func (s *Service) SetAllowLeaveDuringPlay(allow bool) {
	if s == nil {
		return
	}
	s.allowLeaveDuringPlay = allow
}

// SetAutoTimeoutHandler 注册后台托管通知处理器。
func (s *Service) SetAutoTimeoutHandler(fn func(context.Context, string, []Notification)) {
	if s == nil {
		return
	}
	s.onAuto = fn
}

// SetAfterCmdHook 注册"actor 处理完一条命令后"回调，供 BotSupervisor 之类的协作组件接力推进。
// 回调必须非阻塞（应在 goroutine 内完成实际工作），否则会拖慢房间主循环。
func (s *Service) SetAfterCmdHook(fn func(roomID string)) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.onAfterCmd = fn
	s.mu.Unlock()
}

// PlayerIDs 返回房间当前 4 座的 user_id（空座为 ""）。
func (s *Service) PlayerIDs(roomID string) ([4]string, bool) {
	if s == nil || s.lobby == nil {
		return [4]string{}, false
	}
	r, ok := s.lobby.GetRoom(roomID)
	if !ok || r == nil {
		return [4]string{}, false
	}
	var out [4]string
	for i := 0; i < 4 && i < len(r.PlayerIDs); i++ {
		out[i] = r.PlayerIDs[i]
	}
	return out, true
}

// EnsureRoom 若不存在则创建房间并启动该房的 mailbox 协程。
func (s *Service) EnsureRoom(roomID string) error {
	if s == nil {
		return fmt.Errorf("nil service")
	}
	if _, ok := s.lobby.GetRoom(roomID); ok {
		s.ensureActorForExistingRoom(roomID)
		return nil
	}
	r := domainroom.NewRoom(roomID)
	r.MaxHands = s.maxHands
	if err := s.lobby.CreateRoom(roomID, r); err != nil {
		// 并发首进房时，另一协程可能已经抢先建好了房；此时回读并补 actor 即可。
		if _, ok := s.lobby.GetRoom(roomID); ok {
			s.ensureActorForExistingRoom(roomID)
			return nil
		}
		return err
	}
	s.startActorLocked(roomID, r, nil)
	return nil
}

func (s *Service) startActorLocked(roomID string, r *domainroom.Room, initialRound *RoundState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.actors[roomID]; ok {
		return
	}
	if initialRound != nil {
		// 恢复路径注入 clk/tmo，并按当前时刻重锚定 phaseStart：
		// 若 deadline 已过期，scheduler.armUntil 会立即触发 cmdAutoTimeout，
		// 与 ADR-0045 的"重启后超时立即托管"约束一致。
		initialRound.clk = s.clock
		initialRound.tmo = s.tmo
		if initialRound.phaseStartUnixMs == 0 && initialRound.phaseReason != ReasonNone {
			initialRound.phaseStartUnixMs = s.clock.Now().UnixMilli()
			if dur := initialRound.phaseDuration(initialRound.phaseReason, initialRound.surrenderedWaitingSeat(initialRound.phaseReason)); dur > 0 {
				initialRound.deadlineUnixMs = initialRound.phaseStartUnixMs + dur.Milliseconds()
			}
		} else if initialRound.phaseReason != ReasonNone {
			if dur := initialRound.phaseDuration(initialRound.phaseReason, initialRound.surrenderedWaitingSeat(initialRound.phaseReason)); dur > 0 {
				initialRound.deadlineUnixMs = initialRound.phaseStartUnixMs + dur.Milliseconds()
			}
		}
	}
	a := newRoomActorWithCapacity(r, initialRound, s.mailboxCapacity)
	a.engine = s.engine
	a.onExit = s.removeActor
	a.scheduler = newRoomScheduler(roomID, s.clock, a)
	a.onAuto = s.onAuto
	a.onAfterCmd = s.onAfterCmd
	a.allowLeaveDuringPlay = s.allowLeaveDuringPlay
	s.actors[roomID] = a
	go a.run()
}

func (s *Service) ensureActorForExistingRoom(roomID string) {
	s.mu.Lock()
	if _, ok := s.actors[roomID]; ok {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	r, ok := s.lobby.GetRoom(roomID)
	if !ok {
		return
	}
	s.mu.Lock()
	if _, ok := s.actors[roomID]; ok {
		s.mu.Unlock()
		return
	}
	a := newRoomActorWithCapacity(r, nil, s.mailboxCapacity)
	a.engine = s.engine
	a.onExit = s.removeActor
	a.scheduler = newRoomScheduler(roomID, s.clock, a)
	a.onAuto = s.onAuto
	a.onAfterCmd = s.onAfterCmd
	a.allowLeaveDuringPlay = s.allowLeaveDuringPlay
	s.actors[roomID] = a
	s.mu.Unlock()
	go a.run()
}

func (s *Service) removeActor(roomID string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.actors, roomID)
}

func (s *Service) getActor(roomID string) *roomActor {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.actors[roomID]
}

// Join 自动占座并返回座位号。
func (s *Service) Join(ctx context.Context, roomID, userID string) (int, error) {
	if err := s.EnsureRoom(roomID); err != nil {
		return -1, err
	}
	a := s.getActor(roomID)
	if a == nil {
		return -1, fmt.Errorf("room missing: %s", roomID)
	}
	return a.submitJoin(ctx, userID)
}

// Ready 标记准备并尝试开局。
// 返回值：非空载荷表示须在调用方写完准备应答帧之后再调用 Hub.Broadcast，避免与同一
// WebSocket 连接上的其它写操作并发（gorilla/websocket 要求单写者）。
func (s *Service) Ready(ctx context.Context, roomID, userID string) ([]Notification, error) {
	a := s.getActor(roomID)
	if a == nil {
		return nil, fmt.Errorf("room not found")
	}
	return a.submitReady(ctx, userID)
}

// Leave 将玩家从 waiting/ready 房间移除；playing 及以后状态返回错误。
func (s *Service) Leave(ctx context.Context, roomID, userID string) error {
	a := s.getActor(roomID)
	if a == nil {
		return fmt.Errorf("room not found")
	}
	return a.submitLeave(ctx, userID)
}

// Discard 提交当前轮次出牌动作；tok 为客户端阶段令牌，nil 时跳过 drift 校验（旧客户端兼容）。
func (s *Service) Discard(ctx context.Context, roomID, userID, tile string, tok *PhaseToken) ([]Notification, error) {
	a := s.getActor(roomID)
	if a == nil {
		return nil, fmt.Errorf("room not found")
	}
	return a.submitDiscard(ctx, userID, tile, tok)
}

// Pong 提交弃牌抢答窗口中的碰牌动作。
func (s *Service) Pong(ctx context.Context, roomID, userID string, tok *PhaseToken) ([]Notification, error) {
	a := s.getActor(roomID)
	if a == nil {
		return nil, fmt.Errorf("room not found")
	}
	return a.submitPong(ctx, userID, tok)
}

// Chi 提交弃牌抢答窗口中的吃牌动作。默认四川血战规则不会开放该动作。
func (s *Service) Chi(ctx context.Context, roomID, userID string, tiles []string, tok *PhaseToken) ([]Notification, error) {
	a := s.getActor(roomID)
	if a == nil {
		return nil, fmt.Errorf("room not found")
	}
	return a.submitChi(ctx, userID, tiles, tok)
}

// Gang 提交弃牌抢答窗口中的杠牌或当前座位自杠动作。
func (s *Service) Gang(ctx context.Context, roomID, userID, tile string, tok *PhaseToken) ([]Notification, error) {
	a := s.getActor(roomID)
	if a == nil {
		return nil, fmt.Errorf("room not found")
	}
	return a.submitGang(ctx, userID, tile, tok)
}

// Hu 提交胡牌动作（当前为自摸待决窗口）。
func (s *Service) Hu(ctx context.Context, roomID, userID string, tok *PhaseToken) ([]Notification, error) {
	a := s.getActor(roomID)
	if a == nil {
		return nil, fmt.Errorf("room not found")
	}
	return a.submitHu(ctx, userID, tok)
}

// Pass 放弃当前抢答或自摸选择，并由服务端推进下一等待态。
func (s *Service) Pass(ctx context.Context, roomID, userID string, tok *PhaseToken) ([]Notification, error) {
	a := s.getActor(roomID)
	if a == nil {
		return nil, fmt.Errorf("room not found")
	}
	return a.submitPass(ctx, userID, tok)
}

// AutoTimeout 执行当前等待态的服务端托管动作，供上层定时器到期后调用。
func (s *Service) AutoTimeout(ctx context.Context, roomID string) ([]Notification, error) {
	a := s.getActor(roomID)
	if a == nil {
		return nil, fmt.Errorf("room not found")
	}
	return a.submitAutoTimeout(ctx)
}

func (s *Service) OpeningAction(ctx context.Context, roomID, userID, action string, tiles []string, direction, suit int32, params map[string]string, tok *PhaseToken) ([]Notification, error) {
	a := s.getActor(roomID)
	if a == nil {
		return nil, fmt.Errorf("room not found")
	}
	return a.submitOpeningAction(ctx, userID, action, tiles, direction, suit, params, tok)
}

// RoundPersistSnapshot 返回当前进行中牌局的最小可恢复快照。
func (s *Service) RoundPersistSnapshot(ctx context.Context, roomID string) ([]byte, error) {
	a := s.getActor(roomID)
	if a == nil {
		return nil, nil
	}
	return a.submitRoundSnapJSON(ctx)
}

// RoundView 返回当前进行中局面的等待态摘要。
func (s *Service) RoundView(ctx context.Context, roomID string) (RoundView, bool, error) {
	a := s.getActor(roomID)
	if a == nil {
		return RoundView{}, false, nil
	}
	return a.submitRoundView(ctx)
}

// RecoverRoom 基于 Redis snapmeta 恢复房间基础内存态，并重新挂起 actor。
func (s *Service) RecoverRoom(roomID string, playerIDs []string, fsmState string, roundPersistJSON []byte) error {
	if s == nil || s.lobby == nil {
		return fmt.Errorf("nil service")
	}
	if roomID == "" {
		return fmt.Errorf("empty room_id")
	}
	if _, ok := s.lobby.GetRoom(roomID); ok {
		s.ensureActorForExistingRoom(roomID)
		return nil
	}
	r := domainroom.NewRoom(roomID)
	r.MaxHands = s.maxHands
	for _, userID := range playerIDs {
		if userID == "" {
			continue
		}
		if _, ok := r.JoinAutoSeat(userID); !ok {
			return fmt.Errorf("recover room %s: %w", roomID, ErrRoomFull)
		}
	}
	if err := restoreFSMForRecover(r, fsmState); err != nil {
		return fmt.Errorf("recover room %s: %w", roomID, err)
	}
	if err := s.lobby.CreateRoom(roomID, r); err != nil {
		return err
	}
	var initialRound *RoundState
	if domainroom.State(fsmState) == domainroom.StatePlaying && len(roundPersistJSON) > 0 {
		rs, err := RestoreRoundFromPersistJSON(roomID, roundPersistJSON)
		if err != nil {
			return fmt.Errorf("restore round: %w", err)
		}
		initialRound = rs
	} else if domainroom.State(fsmState) == domainroom.StatePlaying {
		return fmt.Errorf("recover room %s: missing round snapshot for playing state", roomID)
	}
	s.startActorLocked(roomID, r, initialRound)
	return nil
}

// restoreFSMForRecover 把 RecoverRoom 提供的 fsmState 字符串映射为 FSM 直接置位。
//
// 空字符串与 "waiting" 视为新房刚建好（NewRoom 默认 idle，等价 waiting），无需置位；
// 其他可识别状态走 FSM.Restore 一次性置位；未知字符串视为持久化错误。
func restoreFSMForRecover(r *domainroom.Room, fsmState string) error {
	switch domainroom.State(fsmState) {
	case "", domainroom.StateWaiting:
		return nil
	case domainroom.StateReady, domainroom.StatePlaying, domainroom.StateSettling, domainroom.StateClosed:
		return r.FSM.Restore(domainroom.State(fsmState))
	default:
		return fmt.Errorf("unknown room state: %q", fsmState)
	}
}

// RoomSnapshot 返回当前内存房间的玩家列表、准备状态与 FSM 状态字符串，供快照与 Redis 元数据写入。
func (s *Service) RoomSnapshot(roomID string) (playerIDs []string, fsmState string, ready [4]bool, ok bool) {
	if s == nil || s.lobby == nil {
		return nil, "", [4]bool{}, false
	}
	if a := s.getActor(roomID); a != nil {
		players, state, ready, err := a.submitRoomSnapshot(context.Background())
		if err == nil {
			return players, state, ready, true
		}
		return nil, "", [4]bool{}, false
	}
	r, exists := s.lobby.GetRoom(roomID)
	if !exists || r == nil {
		return nil, "", [4]bool{}, false
	}
	out := append([]string(nil), r.PlayerIDs[:]...)
	st := ""
	if r.FSM != nil {
		st = string(r.FSM.State())
	}
	return out, st, r.Ready, true
}

// RuleID 返回当前房间服务使用的规则 ID，供持久化摘要写入。
func (s *Service) RuleID() string {
	if s == nil || s.engine == nil {
		return ""
	}
	return s.engine.ruleID
}

// NewUserID 生成用户 ID（登录用）。
func NewUserID() string {
	return uuid.NewString()
}
