package room

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	domainroom "racoo.cn/lsp/internal/domain/room"
	"racoo.cn/lsp/internal/metrics"
)

// ErrRateLimited 表示入口或房间队列限流。
var ErrRateLimited = errors.New("rate limited")

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
	submitMu  sync.Mutex
	closed    atomic.Bool
	onExit    func(roomID string)
	engine    *Engine
	scheduler *roomScheduler
	onAuto    func(context.Context, string, []Notification)
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
	userID string
	tile   string
	res    chan actionResult
}

type cmdPong struct {
	userID string
	res    chan actionResult
}

type cmdGang struct {
	userID string
	tile   string
	res    chan actionResult
}

type cmdHu struct {
	userID string
	res    chan actionResult
}

type cmdAutoTimeout struct {
	res chan actionResult
}

type cmdExchangeThree struct {
	userID    string
	tiles     []string
	direction int32
	res       chan actionResult
}

type cmdQueMen struct {
	userID string
	suit   int32
	res    chan actionResult
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

type roundViewResult struct {
	view RoundView
	ok   bool
}

func newRoomActor(r *domainroom.Room, initialRound *RoundState) *roomActor {
	if r == nil {
		return nil
	}
	return &roomActor{
		room:         r,
		initialRound: initialRound,
		ch:           make(chan any, defaultMailboxCapacity),
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
		case cmdDiscard:
			notifications, err := a.doDiscard(m.userID, m.tile)
			a.resetScheduler()
			m.res <- actionResult{notifications: notifications, err: err}
		case cmdPong:
			notifications, err := a.doPong(m.userID)
			a.resetScheduler()
			m.res <- actionResult{notifications: notifications, err: err}
		case cmdGang:
			notifications, err := a.doGang(m.userID, m.tile)
			a.resetScheduler()
			m.res <- actionResult{notifications: notifications, err: err}
		case cmdHu:
			notifications, err := a.doHu(m.userID)
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
		case cmdExchangeThree:
			notifications, err := a.doExchangeThree(m.userID, m.tiles, m.direction)
			a.resetScheduler()
			m.res <- actionResult{notifications: notifications, err: err}
		case cmdQueMen:
			notifications, err := a.doQueMen(m.userID, m.suit)
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
		default:
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

func (a *roomActor) doJoin(userID string) (int, error) {
	if a.room == nil {
		return -1, fmt.Errorf("nil room")
	}
	seat, ok := a.room.JoinAutoSeat(userID)
	if !ok {
		return -1, fmt.Errorf("room full")
	}
	return seat, nil
}

func (a *roomActor) doReady(userID string) ([]Notification, error) {
	if a.room == nil {
		return nil, fmt.Errorf("nil room")
	}
	r := a.room
	seat := -1
	for i := 0; i < 4; i++ {
		if r.PlayerIDs[i] == userID {
			seat = i
			break
		}
	}
	if seat < 0 {
		return nil, fmt.Errorf("not in room")
	}
	if err := r.SetReady(seat, true); err != nil {
		return nil, err
	}
	if r.FSM.State() == domainroom.StateReady {
		if err := r.StartPlaying(); err != nil {
			return nil, err
		}
		if a.engine == nil {
			return nil, fmt.Errorf("nil engine")
		}
		if a.round != nil {
			return nil, nil
		}
		round, notifications, err := a.engine.StartRound(context.Background(), r.ID, r.PlayerIDs)
		if err != nil {
			return nil, err
		}
		a.round = round
		if a.round != nil && a.round.closed {
			a.closeRoomAfterRound()
		}
		return notifications, nil
	}
	return nil, nil
}

func (a *roomActor) doLeave(userID string) error {
	if a.room == nil {
		return fmt.Errorf("nil room")
	}
	return a.room.Leave(userID)
}

func (a *roomActor) doDiscard(userID, tile string) ([]Notification, error) {
	seat, err := a.seatOf(userID)
	if err != nil {
		return nil, err
	}
	notifications, err := a.engine.ApplyDiscard(context.Background(), a.round, seat, tile)
	if err != nil {
		return nil, err
	}
	if a.round.closed {
		a.closeRoomAfterRound()
		a.round = nil
	}
	return notifications, nil
}

func (a *roomActor) doPong(userID string) ([]Notification, error) {
	seat, err := a.seatOf(userID)
	if err != nil {
		return nil, err
	}
	notifications, err := a.engine.ApplyPong(context.Background(), a.round, seat)
	if err != nil {
		return nil, err
	}
	if a.round.closed {
		a.closeRoomAfterRound()
		a.round = nil
	}
	return notifications, nil
}

func (a *roomActor) doGang(userID, tile string) ([]Notification, error) {
	seat, err := a.seatOf(userID)
	if err != nil {
		return nil, err
	}
	notifications, err := a.engine.ApplyGang(context.Background(), a.round, seat, tile)
	if err != nil {
		return nil, err
	}
	if a.round.closed {
		a.closeRoomAfterRound()
		a.round = nil
	}
	return notifications, nil
}

func (a *roomActor) doHu(userID string) ([]Notification, error) {
	seat, err := a.seatOf(userID)
	if err != nil {
		return nil, err
	}
	notifications, err := a.engine.ApplyHu(context.Background(), a.round, seat)
	if err != nil {
		return nil, err
	}
	if a.round.closed {
		a.closeRoomAfterRound()
		a.round = nil
	}
	return notifications, nil
}

func (a *roomActor) doAutoTimeout() ([]Notification, error) {
	notifications, err := a.engine.ApplyTimeout(context.Background(), a.round)
	if err != nil {
		return nil, err
	}
	if a.round.closed {
		a.closeRoomAfterRound()
		a.round = nil
	}
	return notifications, nil
}

func (a *roomActor) doExchangeThree(userID string, tiles []string, direction int32) ([]Notification, error) {
	seat, err := a.seatOf(userID)
	if err != nil {
		return nil, err
	}
	return a.engine.ApplyExchangeThree(context.Background(), a.round, seat, tiles, direction)
}

func (a *roomActor) doQueMen(userID string, suit int32) ([]Notification, error) {
	seat, err := a.seatOf(userID)
	if err != nil {
		return nil, err
	}
	return a.engine.ApplyQueMen(context.Background(), a.round, seat, suit)
}

func (a *roomActor) seatOf(userID string) (int, error) {
	if a.room == nil {
		return -1, fmt.Errorf("nil room")
	}
	if a.round == nil {
		return -1, fmt.Errorf("round not started")
	}
	for i := 0; i < 4; i++ {
		if a.room.PlayerIDs[i] == userID {
			return i, nil
		}
	}
	return -1, fmt.Errorf("not in room")
}

func (a *roomActor) closeRoomAfterRound() {
	if a == nil || a.room == nil {
		return
	}
	_ = a.room.CloseToSettling()
	_ = a.room.CloseRoom()
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

func (a *roomActor) submitDiscard(ctx context.Context, userID, tile string) ([]Notification, error) {
	return a.submitAction(ctx, cmdDiscard{userID: userID, tile: tile, res: make(chan actionResult, 1)})
}

func (a *roomActor) submitPong(ctx context.Context, userID string) ([]Notification, error) {
	return a.submitAction(ctx, cmdPong{userID: userID, res: make(chan actionResult, 1)})
}

func (a *roomActor) submitGang(ctx context.Context, userID, tile string) ([]Notification, error) {
	return a.submitAction(ctx, cmdGang{userID: userID, tile: tile, res: make(chan actionResult, 1)})
}

func (a *roomActor) submitHu(ctx context.Context, userID string) ([]Notification, error) {
	return a.submitAction(ctx, cmdHu{userID: userID, res: make(chan actionResult, 1)})
}

func (a *roomActor) submitAutoTimeout(ctx context.Context) ([]Notification, error) {
	return a.submitAction(ctx, cmdAutoTimeout{res: make(chan actionResult, 1)})
}

func (a *roomActor) submitExchangeThree(ctx context.Context, userID string, tiles []string, direction int32) ([]Notification, error) {
	return a.submitAction(ctx, cmdExchangeThree{userID: userID, tiles: append([]string(nil), tiles...), direction: direction, res: make(chan actionResult, 1)})
}

func (a *roomActor) submitQueMen(ctx context.Context, userID string, suit int32) ([]Notification, error) {
	return a.submitAction(ctx, cmdQueMen{userID: userID, suit: suit, res: make(chan actionResult, 1)})
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
	case cmdGang:
		rr := <-c.res
		return rr.notifications, rr.err
	case cmdHu:
		rr := <-c.res
		return rr.notifications, rr.err
	case cmdAutoTimeout:
		rr := <-c.res
		return rr.notifications, rr.err
	case cmdExchangeThree:
		rr := <-c.res
		return rr.notifications, rr.err
	case cmdQueMen:
		rr := <-c.res
		return rr.notifications, rr.err
	default:
		return nil, fmt.Errorf("unsupported action command")
	}
}
