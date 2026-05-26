package room

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	domainroom "racoo.cn/lsp/internal/domain/room"
	"racoo.cn/lsp/internal/metrics"
	"racoo.cn/lsp/pkg/logx"
)

// ErrRateLimited 表示入口或房间队列限流。
var ErrRateLimited = errors.New("rate limited")

// ErrRoomFull 表示房间已无空余座位。
var ErrRoomFull = errors.New("room full")

const defaultMailboxCapacity = 64

// roomActor 单房间串行化执行 Join/Ready 等命令，符合「每房一事件循环」模型。
type roomActor struct {
	room *domainroom.Room
	// initialRound 用于冷启动恢复进行中的牌局。
	initialRound *RoundState
	round        *RoundState
	// 当前实现保持“单房单命令在途”，避免房间关闭时遗留未消费命令造成悬挂。
	ch chan any
	// submitMu 串行化外部提交，保证房间关闭后不会再有新的发送者卡在无人接收的通道上。
	submitMu             sync.Mutex
	closed               atomic.Bool
	onExit               func(roomID string)
	engine               *Engine
	scheduler            *roomScheduler
	onAuto               func(context.Context, string, []Notification)
	onAfterCmd           func(roomID string)
	allowLeaveDuringPlay bool
}

type cmdJoin struct {
	userID string
	res    chan joinResult
}

type joinResult struct {
	seat int
	err  error
}

type cmdReady struct {
	userID string
	res    chan readyResult
}

type readyResult struct {
	notifications []Notification
	err           error
}

type cmdLeave struct {
	userID string
	res    chan error
}

type cmdDiscard struct {
	userID   string
	tile     string
	phaseTok *PhaseToken
	ctx      context.Context
	res      chan actionResult
}

type cmdPong struct {
	userID   string
	phaseTok *PhaseToken
	ctx      context.Context
	res      chan actionResult
}

type cmdChi struct {
	userID   string
	tiles    []string
	phaseTok *PhaseToken
	ctx      context.Context
	res      chan actionResult
}

type cmdGang struct {
	userID   string
	tile     string
	phaseTok *PhaseToken
	ctx      context.Context
	res      chan actionResult
}

type cmdHu struct {
	userID   string
	phaseTok *PhaseToken
	ctx      context.Context
	res      chan actionResult
}

type cmdPass struct {
	userID   string
	phaseTok *PhaseToken
	ctx      context.Context
	res      chan actionResult
}

type cmdAutoTimeout struct {
	ctx context.Context
	res chan actionResult
}

type cmdOpeningAction struct {
	userID    string
	action    string
	tiles     []string
	direction int32
	suit      int32
	params    map[string]string
	phaseTok  *PhaseToken
	ctx       context.Context
	res       chan actionResult
}

type actionResult struct {
	notifications []Notification
	err           error
}

type cmdRoundSnap struct {
	res chan roundSnapResult
}

type roundSnapResult struct {
	data []byte
	err  error
}

type cmdRoundView struct {
	res chan roundViewResult
}

type cmdRoomSnapshot struct {
	res chan roomSnapshotResult
}

type roomSnapshotResult struct {
	playerIDs []string
	fsmState  string
	ready     [4]bool
}

type roundViewResult struct {
	view RoundView
	ok   bool
}

func newRoomActor(r *domainroom.Room, initialRound *RoundState) *roomActor {
	return newRoomActorWithCapacity(r, initialRound, defaultMailboxCapacity)
}

func newRoomActorWithCapacity(r *domainroom.Room, initialRound *RoundState, capacity int) *roomActor {
	if r == nil {
		return nil
	}
	if capacity <= 0 {
		capacity = defaultMailboxCapacity
	}
	return &roomActor{
		room:                 r,
		initialRound:         initialRound,
		ch:                   make(chan any, capacity),
		allowLeaveDuringPlay: true,
	}
}

// run 为唯一消费者，所有对 *Room 的变更必须在此协程中完成。
func (a *roomActor) run() {
	if a == nil {
		return
	}
	if a.initialRound != nil {
		a.round = a.initialRound
		a.initialRound = nil
	}
	a.resetScheduler()
	for msg := range a.ch {
		if a.room != nil {
			metrics.ActorQueueDepth.WithLabelValues(a.room.ID).Set(float64(len(a.ch)))
		}
		switch m := msg.(type) {
		case cmdJoin:
			seat, err := a.doJoin(m.userID)
			m.res <- joinResult{seat: seat, err: err}
		case cmdReady:
			notifications, err := a.doReady(m.userID)
			a.resetScheduler()
			m.res <- readyResult{notifications: notifications, err: err}
		case cmdLeave:
			m.res <- a.doLeave(m.userID)
			a.resetScheduler()
		case cmdDiscard:
			if err := a.checkPhaseToken(m.phaseTok); err != nil {
				a.logActionRejected(m.ctx, m.userID, "discard", m.phaseTok, err)
				m.res <- actionResult{err: err}
				continue
			}
			notifications, err := a.doDiscard(m.userID, m.tile)
			if err != nil {
				a.logActionRejected(m.ctx, m.userID, "discard", m.phaseTok, err)
			}
			a.resetScheduler()
			m.res <- actionResult{notifications: notifications, err: err}
		case cmdPong:
			if err := a.checkPhaseToken(m.phaseTok); err != nil {
				a.logActionRejected(m.ctx, m.userID, "pong", m.phaseTok, err)
				m.res <- actionResult{err: err}
				continue
			}
			notifications, err := a.doPong(m.userID)
			if err != nil {
				a.logActionRejected(m.ctx, m.userID, "pong", m.phaseTok, err)
			}
			a.resetScheduler()
			m.res <- actionResult{notifications: notifications, err: err}
		case cmdChi:
			if err := a.checkPhaseToken(m.phaseTok); err != nil {
				a.logActionRejected(m.ctx, m.userID, "chi", m.phaseTok, err)
				m.res <- actionResult{err: err}
				continue
			}
			notifications, err := a.doChi(m.userID, m.tiles)
			if err != nil {
				a.logActionRejected(m.ctx, m.userID, "chi", m.phaseTok, err)
			}
			a.resetScheduler()
			m.res <- actionResult{notifications: notifications, err: err}
		case cmdGang:
			if err := a.checkPhaseToken(m.phaseTok); err != nil {
				a.logActionRejected(m.ctx, m.userID, "gang", m.phaseTok, err)
				m.res <- actionResult{err: err}
				continue
			}
			notifications, err := a.doGang(m.userID, m.tile)
			if err != nil {
				a.logActionRejected(m.ctx, m.userID, "gang", m.phaseTok, err)
			}
			a.resetScheduler()
			m.res <- actionResult{notifications: notifications, err: err}
		case cmdHu:
			if err := a.checkPhaseToken(m.phaseTok); err != nil {
				a.logActionRejected(m.ctx, m.userID, "hu", m.phaseTok, err)
				m.res <- actionResult{err: err}
				continue
			}
			notifications, err := a.doHu(m.userID)
			if err != nil {
				a.logActionRejected(m.ctx, m.userID, "hu", m.phaseTok, err)
			}
			a.resetScheduler()
			m.res <- actionResult{notifications: notifications, err: err}
		case cmdPass:
			if err := a.checkPhaseToken(m.phaseTok); err != nil {
				a.logActionRejected(m.ctx, m.userID, "pass", m.phaseTok, err)
				m.res <- actionResult{err: err}
				continue
			}
			notifications, err := a.doPass(m.userID)
			if err != nil {
				a.logActionRejected(m.ctx, m.userID, "pass", m.phaseTok, err)
			}
			a.resetScheduler()
			m.res <- actionResult{notifications: notifications, err: err}
		case cmdAutoTimeout:
			kind := "none"
			if a.round != nil {
				kind = a.round.waitingKind()
			}
			notifications, err := a.doAutoTimeout()
			if err == nil {
				metrics.AutoTimeoutTotal.WithLabelValues(kind).Inc()
			}
			a.resetScheduler()
			m.res <- actionResult{notifications: notifications, err: err}
		case cmdOpeningAction:
			if err := a.checkPhaseToken(m.phaseTok); err != nil {
				a.logActionRejected(m.ctx, m.userID, m.action, m.phaseTok, err)
				m.res <- actionResult{err: err}
				continue
			}
			notifications, err := a.doOpeningAction(m.userID, m.action, m.tiles, m.direction, m.suit, m.params)
			if err != nil {
				a.logActionRejected(m.ctx, m.userID, m.action, m.phaseTok, err)
			}
			a.resetScheduler()
			m.res <- actionResult{notifications: notifications, err: err}
		case cmdRoundSnap:
			var data []byte
			var err error
			if a.round != nil && !a.round.closed {
				data, err = a.round.MarshalRoundPersistJSON()
			}
			m.res <- roundSnapResult{data: data, err: err}
		case cmdRoundView:
			if a.round == nil || a.round.closed {
				m.res <- roundViewResult{}
				break
			}
			m.res <- roundViewResult{view: a.round.SnapshotView(), ok: true}
		case cmdRoomSnapshot:
			out := append([]string(nil), a.room.PlayerIDs[:]...)
			state := ""
			if a.room.FSM != nil {
				state = string(a.room.FSM.State())
			}
			m.res <- roomSnapshotResult{playerIDs: out, fsmState: state, ready: a.room.Ready}
		default:
		}
		if a.onAfterCmd != nil && a.room != nil {
			a.onAfterCmd(a.room.ID)
		}
		if a.room != nil && a.room.FSM != nil && a.room.FSM.State() == domainroom.StateClosed {
			a.closed.Store(true)
			if a.scheduler != nil {
				a.scheduler.stop()
			}
			if a.onExit != nil {
				a.onExit(a.room.ID)
			}
			return
		}
	}
}

// submitJoin 向房间 actor 提交加入请求并同步等待结果（ctx 可取消防悬挂）。
func (a *roomActor) submitJoin(ctx context.Context, userID string) (int, error) {
	if a == nil {
		return -1, fmt.Errorf("nil actor")
	}
	a.submitMu.Lock()
	defer a.submitMu.Unlock()
	if a.closed.Load() {
		return -1, fmt.Errorf("room closed")
	}
	res := make(chan joinResult, 1)
	cmd := cmdJoin{userID: userID, res: res}
	select {
	case a.ch <- cmd:
	default:
		return -1, ErrRateLimited
	case <-ctx.Done():
		return -1, ctx.Err()
	}
	select {
	case jr := <-res:
		return jr.seat, jr.err
	case <-ctx.Done():
		return -1, ctx.Err()
	}
}

// submitReady 向房间 actor 提交准备请求并同步等待结果。
func (a *roomActor) submitReady(ctx context.Context, userID string) ([]Notification, error) {
	if a == nil {
		return nil, fmt.Errorf("nil actor")
	}
	a.submitMu.Lock()
	defer a.submitMu.Unlock()
	if a.closed.Load() {
		return nil, fmt.Errorf("room closed")
	}
	res := make(chan readyResult, 1)
	cmd := cmdReady{userID: userID, res: res}
	if cap(a.ch) == 0 {
		select {
		case a.ch <- cmd:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	} else {
		select {
		case a.ch <- cmd:
		default:
			return nil, ErrRateLimited
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	select {
	case rr := <-res:
		return rr.notifications, rr.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (a *roomActor) submitLeave(ctx context.Context, userID string) error {
	if a == nil {
		return fmt.Errorf("nil actor")
	}
	a.submitMu.Lock()
	defer a.submitMu.Unlock()
	if a.closed.Load() {
		return fmt.Errorf("room closed")
	}
	res := make(chan error, 1)
	cmd := cmdLeave{userID: userID, res: res}
	select {
	case a.ch <- cmd:
	default:
		return ErrRateLimited
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-res:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (a *roomActor) submitDiscard(ctx context.Context, userID, tile string, tok *PhaseToken) ([]Notification, error) {
	return a.submitAction(ctx, cmdDiscard{userID: userID, tile: tile, phaseTok: tok, ctx: ctx, res: make(chan actionResult, 1)})
}

func (a *roomActor) submitPong(ctx context.Context, userID string, tok *PhaseToken) ([]Notification, error) {
	return a.submitAction(ctx, cmdPong{userID: userID, phaseTok: tok, ctx: ctx, res: make(chan actionResult, 1)})
}

func (a *roomActor) submitChi(ctx context.Context, userID string, tiles []string, tok *PhaseToken) ([]Notification, error) {
	return a.submitAction(ctx, cmdChi{userID: userID, tiles: tiles, phaseTok: tok, ctx: ctx, res: make(chan actionResult, 1)})
}

func (a *roomActor) submitGang(ctx context.Context, userID, tile string, tok *PhaseToken) ([]Notification, error) {
	return a.submitAction(ctx, cmdGang{userID: userID, tile: tile, phaseTok: tok, ctx: ctx, res: make(chan actionResult, 1)})
}

func (a *roomActor) submitHu(ctx context.Context, userID string, tok *PhaseToken) ([]Notification, error) {
	return a.submitAction(ctx, cmdHu{userID: userID, phaseTok: tok, ctx: ctx, res: make(chan actionResult, 1)})
}

func (a *roomActor) submitPass(ctx context.Context, userID string, tok *PhaseToken) ([]Notification, error) {
	return a.submitAction(ctx, cmdPass{userID: userID, phaseTok: tok, ctx: ctx, res: make(chan actionResult, 1)})
}

// checkPhaseToken 是 actor 状态变更动作的统一前置校验；详见 ADR-0045。
// 调用时必须已位于 actor 单 goroutine 内（即在 run() 的 switch 中），
// 以保证 (rs.step, rs.phaseReason) 读取与后续 do* 写入是同一时刻的原子组合。
func (a *roomActor) checkPhaseToken(tok *PhaseToken) error {
	if a == nil || a.round == nil {
		return nil
	}
	return a.round.validatePhaseToken(tok)
}

func (a *roomActor) submitAutoTimeout(ctx context.Context) ([]Notification, error) {
	return a.submitAction(ctx, cmdAutoTimeout{ctx: ctx, res: make(chan actionResult, 1)})
}

func (a *roomActor) submitOpeningAction(ctx context.Context, userID, action string, tiles []string, direction, suit int32, params map[string]string, tok *PhaseToken) ([]Notification, error) {
	return a.submitAction(ctx, cmdOpeningAction{
		userID:    userID,
		action:    action,
		tiles:     append([]string(nil), tiles...),
		direction: direction,
		suit:      suit,
		params:    cloneStringMap(params),
		phaseTok:  tok,
		ctx:       ctx,
		res:       make(chan actionResult, 1),
	})
}

// logActionRejected 在 actor 单协程内统一记录动作拒绝日志，覆盖 token 漂移与状态不满足。
func (a *roomActor) logActionRejected(ctx context.Context, userID, action string, tok *PhaseToken, err error) {
	if err == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if a != nil && a.room != nil {
		ctx = logx.WithRoomID(ctx, a.room.ID)
	}
	if userID != "" {
		ctx = logx.WithUserID(ctx, userID)
	}
	phaseStep := int64(0)
	phaseReason := "none"
	if a != nil && a.round != nil {
		phaseStep = int64(a.round.step)
		phaseReason = a.round.phaseReason.String()
	}
	drift := false
	tokenStep := int64(0)
	tokenReason := "none"
	if tok != nil {
		tokenStep = tok.Step
		tokenReason = tok.Reason.String()
	}
	var driftErr *PhaseDriftError
	if errors.As(err, &driftErr) {
		drift = true
	}
	logx.Warn(ctx, "房间动作被拒绝",
		"action", action,
		"phase_step", phaseStep,
		"phase_reason", phaseReason,
		"token_step", tokenStep,
		"token_reason", tokenReason,
		"phase_drifted", drift,
		"err", err.Error(),
	)
}

func (a *roomActor) submitRoundSnapJSON(ctx context.Context) ([]byte, error) {
	if a == nil {
		return nil, fmt.Errorf("nil actor")
	}
	a.submitMu.Lock()
	defer a.submitMu.Unlock()
	if a.closed.Load() {
		return nil, fmt.Errorf("room closed")
	}
	res := make(chan roundSnapResult, 1)
	cmd := cmdRoundSnap{res: res}
	if cap(a.ch) == 0 {
		select {
		case a.ch <- cmd:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	} else {
		select {
		case a.ch <- cmd:
		default:
			return nil, ErrRateLimited
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	select {
	case rr := <-res:
		return rr.data, rr.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (a *roomActor) submitRoundView(ctx context.Context) (RoundView, bool, error) {
	if a == nil {
		return RoundView{}, false, fmt.Errorf("nil actor")
	}
	a.submitMu.Lock()
	defer a.submitMu.Unlock()
	if a.closed.Load() {
		return RoundView{}, false, fmt.Errorf("room closed")
	}
	res := make(chan roundViewResult, 1)
	cmd := cmdRoundView{res: res}
	select {
	case a.ch <- cmd:
	default:
		return RoundView{}, false, ErrRateLimited
	case <-ctx.Done():
		return RoundView{}, false, ctx.Err()
	}
	select {
	case rr := <-res:
		return rr.view, rr.ok, nil
	case <-ctx.Done():
		return RoundView{}, false, ctx.Err()
	}
}

func (a *roomActor) submitRoomSnapshot(ctx context.Context) ([]string, string, [4]bool, error) {
	if a == nil {
		return nil, "", [4]bool{}, fmt.Errorf("nil actor")
	}
	a.submitMu.Lock()
	defer a.submitMu.Unlock()
	if a.closed.Load() {
		return nil, "", [4]bool{}, fmt.Errorf("room closed")
	}
	res := make(chan roomSnapshotResult, 1)
	cmd := cmdRoomSnapshot{res: res}
	select {
	case a.ch <- cmd:
	default:
		return nil, "", [4]bool{}, ErrRateLimited
	case <-ctx.Done():
		return nil, "", [4]bool{}, ctx.Err()
	}
	select {
	case rr := <-res:
		return rr.playerIDs, rr.fsmState, rr.ready, nil
	case <-ctx.Done():
		return nil, "", [4]bool{}, ctx.Err()
	}
}

func (a *roomActor) submitAction(ctx context.Context, cmd any) ([]Notification, error) {
	if a == nil {
		return nil, fmt.Errorf("nil actor")
	}
	a.submitMu.Lock()
	defer a.submitMu.Unlock()
	if a.closed.Load() {
		return nil, fmt.Errorf("room closed")
	}
	if cap(a.ch) == 0 {
		select {
		case a.ch <- cmd:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	} else {
		select {
		case a.ch <- cmd:
		default:
			return nil, ErrRateLimited
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	switch c := cmd.(type) {
	case cmdDiscard:
		rr := <-c.res
		return rr.notifications, rr.err
	case cmdPong:
		rr := <-c.res
		return rr.notifications, rr.err
	case cmdChi:
		rr := <-c.res
		return rr.notifications, rr.err
	case cmdGang:
		rr := <-c.res
		return rr.notifications, rr.err
	case cmdHu:
		rr := <-c.res
		return rr.notifications, rr.err
	case cmdPass:
		rr := <-c.res
		return rr.notifications, rr.err
	case cmdAutoTimeout:
		rr := <-c.res
		return rr.notifications, rr.err
	case cmdOpeningAction:
		rr := <-c.res
		return rr.notifications, rr.err
	default:
		return nil, fmt.Errorf("unsupported action command")
	}
}
