package room

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	domainroom "racoo.cn/lsp/internal/domain/room"
)

// roomActor 单房间串行化执行 Join/Ready 等命令，符合「每房一事件循环」模型。
type roomActor struct {
	room *domainroom.Room
	// 当前实现保持“单房单命令在途”，避免房间关闭时遗留未消费命令造成悬挂。
	ch chan any
	// submitMu 串行化外部提交，保证房间关闭后不会再有新的发送者卡在无人接收的通道上。
	submitMu sync.Mutex
	closed   atomic.Bool
	onExit   func(roomID string)
	engine   *Engine
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

func newRoomActor(r *domainroom.Room) *roomActor {
	if r == nil {
		return nil
	}
	return &roomActor{
		room: r,
		ch:   make(chan any),
	}
}

// run 为唯一消费者，所有对 *Room 的变更必须在此协程中完成。
func (a *roomActor) run() {
	if a == nil {
		return
	}
	for msg := range a.ch {
		switch m := msg.(type) {
		case cmdJoin:
			seat, err := a.doJoin(m.userID)
			m.res <- joinResult{seat: seat, err: err}
		case cmdReady:
			notifications, err := a.doReady(m.userID)
			m.res <- readyResult{notifications: notifications, err: err}
		default:
		}
		if a.room != nil && a.room.FSM != nil && a.room.FSM.State() == domainroom.StateClosed {
			a.closed.Store(true)
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
		notifications, err := a.engine.PlayAutoRound(context.Background(), r.ID, r.PlayerIDs)
		if err != nil {
			return nil, err
		}
		_ = r.CloseToSettling()
		_ = r.CloseRoom()
		return notifications, nil
	}
	return nil, nil
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
	select {
	case a.ch <- cmd:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case rr := <-res:
		return rr.notifications, rr.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
