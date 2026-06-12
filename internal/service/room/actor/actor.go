package actor

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"racoo.cn/lsp/internal/clock"
	domainroom "racoo.cn/lsp/internal/domain/room"
	"racoo.cn/lsp/internal/metrics"
	eng "racoo.cn/lsp/internal/service/room/engine"
	"racoo.cn/lsp/pkg/logx"
)

// 类型别名，避免调用方引用 engine 包。
type (
	RoundState      = eng.RoundState
	Engine          = eng.Engine
	Notification    = eng.Notification
	PhaseToken      = eng.PhaseToken
	PhaseDriftError = eng.PhaseDriftError
	RoundView       = eng.RoundView
	WaitingReason   = eng.WaitingReason
	Kind            = eng.Kind
	Seat            = eng.Seat
)

const (
	SeatCount   = eng.SeatCount
	SeatInvalid = eng.SeatInvalid
)

// ErrRateLimited 表示入口或房间队列限流。
var ErrRateLimited = errors.New("rate limited")

// ErrRoomFull 表示房间已无空余座位。
var ErrRoomFull = errors.New("room full")

const DefaultMailboxCapacity = 64

// Actor 单房间串行化执行 Join/Ready 等命令，符合「每房一事件循环」模型。
type Actor struct {
	Room *domainroom.Room
	// initialRound 用于冷启动恢复进行中的牌局。
	initialRound *RoundState
	round        *RoundState
	// 当前实现保持"单房单命令在途"，避免房间关闭时遗留未消费命令造成悬挂。
	ch chan any
	// submitMu 串行化外部提交，保证 closed 置位与新发送之间无竞态。
	// 注意：Submit* 函数在成功发送到 ch 后立即释放锁，不持锁等待响应，
	// 以避免与 run() 关闭时的排空操作产生死锁。
	submitMu             sync.Mutex
	closed               atomic.Bool
	onExit               func(roomID string)
	engine               *Engine
	scheduler            *scheduler
	onAuto               func(context.Context, string, []Notification)
	onAfterCmd           func(roomID string)
	allowLeaveDuringPlay bool
	// offlineTimers 记录各座位的离线投降计时器；键为 userID。
	// 全部操作均在 actor run goroutine 内串行执行，无需额外同步。
	offlineTimers map[string]*time.Timer
	// offlineTimerGens 记录每个 userID 的定时器代号，用于作废旧回调。
	// 全部操作均在 actor run goroutine 内串行执行，无需额外同步。
	offlineTimerGens map[string]uint64
	// offlineSurrenderAfter 为离线投降延迟；零值使用 DefaultOfflineSurrenderAfter。
	offlineSurrenderAfter time.Duration
	// draining 在 cmdShutdownTimers 处理后置位，阻止离线投降定时器再被创建；
	// 仅在 actor run goroutine 内读写，无需额外同步。
	draining bool
}

// DefaultOfflineSurrenderAfter 是离线投降的默认等待时长。
const DefaultOfflineSurrenderAfter = 30 * time.Second

// cmdMarkOffline 通知 actor 某座位玩家已离线，actor 内部启动投降定时器。
// 投降决策权在 actor，消除 Gate 层 IsRegistered+ApplyEvent 的 TOCTOU 竞争。
type cmdMarkOffline struct {
	userID string
}

// cmdCancelOffline 通知 actor 玩家已重连，取消之前的投降定时器（fire-and-forget）。
type cmdCancelOffline struct {
	userID string
}

// cmdOfflineTimeout 是离线投降定时器到期后直接投递到 ch 的消息。
// AfterFunc 回调不调用任何 Submit*，只投递此消息，保证全部逻辑在 actor 串行上下文内执行。
// gen 用于作废重置前已创建的旧定时器回调。
type cmdOfflineTimeout struct {
	userID string
	gen    uint64
}

// cmdShutdownTimers 在进程优雅停机时停止本房间全部自驱动定时器（阶段超时与离线投降）。
type cmdShutdownTimers struct {
	res chan struct{}
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

// New 创建 Actor 并装配 Config，但不启动事件循环；调用方须调用 Run()。
func New(room *domainroom.Room, initialRound *RoundState, cfg Config) *Actor {
	if room == nil {
		return nil
	}
	capacity := cfg.Capacity
	if capacity <= 0 {
		capacity = DefaultMailboxCapacity
	}
	clk := cfg.Clock
	if clk == nil {
		clk = clock.NewReal()
	}
	a := &Actor{
		Room:                  room,
		initialRound:          initialRound,
		ch:                    make(chan any, capacity),
		engine:                cfg.Engine,
		onExit:                cfg.OnExit,
		onAuto:                cfg.OnAutoTimeout,
		onAfterCmd:            cfg.OnAfterCmd,
		allowLeaveDuringPlay:  cfg.AllowLeaveDuringPlay,
		offlineSurrenderAfter: cfg.OfflineSurrenderAfter,
	}
	a.scheduler = newRoomScheduler(room.ID, clk, a)
	return a
}

// MailboxCap 返回邮箱 channel 容量，供测试检查配置是否生效。
func (a *Actor) MailboxCap() int {
	if a == nil {
		return 0
	}
	return cap(a.ch)
}

// Run 为唯一消费者，所有对 *Room 的变更必须在此协程中完成。
func (a *Actor) Run() {
	if a == nil {
		return
	}
	if a.initialRound != nil {
		a.round = a.initialRound
		a.initialRound = nil
	}
	a.resetScheduler()
	for msg := range a.ch {
		if a.Room != nil {
			metrics.ActorQueueDepth.WithLabelValues(a.Room.ID).Set(float64(len(a.ch)))
		}
		switch m := msg.(type) {
		case cmdJoin:
			seat, err := a.doJoin(m.userID)
			m.res <- joinResult{seat: seat, err: err}
		case cmdReady:
			a.cancelOfflineTimer(m.userID)
			notifications, err := a.doReady(m.userID)
			a.resetScheduler()
			m.res <- readyResult{notifications: notifications, err: err}
		case cmdLeave:
			a.cancelOfflineTimer(m.userID)
			m.res <- a.doLeave(m.userID)
			a.resetScheduler()
		case cmdDiscard:
			a.cancelOfflineTimer(m.userID)
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
			a.cancelOfflineTimer(m.userID)
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
			a.cancelOfflineTimer(m.userID)
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
			a.cancelOfflineTimer(m.userID)
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
			a.cancelOfflineTimer(m.userID)
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
			a.cancelOfflineTimer(m.userID)
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
				kind = a.round.WaitingKind()
			}
			notifications, err := a.doAutoTimeout()
			if err == nil {
				metrics.AutoTimeoutTotal.WithLabelValues(kind).Inc()
			}
			a.resetScheduler()
			m.res <- actionResult{notifications: notifications, err: err}
		case cmdOpeningAction:
			a.cancelOfflineTimer(m.userID)
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
			if a.round != nil && !a.round.IsClosed() {
				data, err = a.round.MarshalRoundPersistJSON()
			}
			m.res <- roundSnapResult{data: data, err: err}
		case cmdRoundView:
			if a.round == nil || a.round.IsClosed() {
				m.res <- roundViewResult{}
				break
			}
			m.res <- roundViewResult{view: a.round.SnapshotView(), ok: true}
		case cmdRoomSnapshot:
			out := append([]string(nil), a.Room.PlayerIDs[:]...)
			state := ""
			if a.Room.FSM != nil {
				state = string(a.Room.FSM.State())
			}
			m.res <- roomSnapshotResult{playerIDs: out, fsmState: state, ready: a.Room.Ready}
		case cmdMarkOffline:
			// 玩家离线：启动投降计时器。已有计时器则先重置（防止同一连接多次触发）。
			a.ensureOfflineTimer(m.userID)
		case cmdCancelOffline:
			// 玩家重连：取消计时器（fire-and-forget，不需要响应）。
			a.cancelOfflineTimer(m.userID)
		case cmdShutdownTimers:
			// 优雅停机：置 draining 阻止新定时器，停掉已有定时器；
			// 已在途的超时回调由 scheduler.waitInflight 在 actor 循环外等待。
			a.draining = true
			for _, t := range a.offlineTimers {
				if t != nil {
					t.Stop()
				}
			}
			a.offlineTimers = nil
			a.offlineTimerGens = nil
			if a.scheduler != nil {
				a.scheduler.markDraining()
			}
			m.res <- struct{}{}
		case cmdOfflineTimeout:
			// 定时器到期事件：在串行上下文内检查代号，过期回调直接忽略。
			if a.offlineTimerGens[m.userID] == m.gen {
				if err := a.doLeave(m.userID); err != nil {
					ctx := logx.WithUserID(context.Background(), m.userID)
					if a.Room != nil {
						ctx = logx.WithRoomID(ctx, a.Room.ID)
					}
					logx.Warn(ctx, "离线投降 Leave 失败", "err", err.Error())
				}
				a.resetScheduler()
			}
		default:
			// 到达此分支意味着某处向 actor 信道投递了未注册的命令类型，属于编程错误。
			// panic 在开发期可立即暴露问题；生产环境中进程崩溃优于静默消费错误消息。
			panic(fmt.Sprintf("actor 收到未处理的命令类型: %T", msg))
		}
		if a.onAfterCmd != nil && a.Room != nil {
			a.onAfterCmd(a.Room.ID)
		}
		if a.Room != nil && a.Room.FSM != nil && a.Room.FSM.State() == domainroom.StateClosed {
			// 在 submitMu 保护下置位，保证此后所有 Submit* 调用都能观察到 closed=true。
			a.submitMu.Lock()
			a.closed.Store(true)
			a.submitMu.Unlock()

			// 排空 ch：对所有在途命令回写"房间已关闭"错误，解除等待 <-res 的 goroutine。
			// 经 FIX-1，Submit* 发送成功后立即释放 submitMu，此处排空不会与锁竞争。
			// 必须用 comma-ok 区分"信道已被外部关闭"：已关闭信道的接收永远立即就绪，
			// 缺少该检查会在 drain 中无限读出零值自旋，Run 永不退出。
		drain:
			for {
				select {
				case pending, ok := <-a.ch:
					if !ok {
						break drain
					}
					a.rejectPendingMsg(pending)
				default:
					break drain
				}
			}

			// 清理所有离线投降定时器。
			for _, t := range a.offlineTimers {
				if t != nil {
					t.Stop()
				}
			}
			a.offlineTimers = nil
			a.offlineTimerGens = nil
			if a.scheduler != nil {
				a.scheduler.stop()
			}
			if a.onExit != nil {
				a.onExit(a.Room.ID)
			}
			return
		}
	}
}

// errRoomClosed 是房间关闭时统一回写给在途命令的哨兵错误。
var errRoomClosed = errors.New("room closed")

// rejectPendingMsg 对带 res channel 的命令回写"房间已关闭"错误。
// 仅在 run() 排空 ch 时调用，保证等待 <-res 的 goroutine 能正常解除阻塞。
func (a *Actor) rejectPendingMsg(msg any) {
	switch m := msg.(type) {
	case cmdJoin:
		m.res <- joinResult{seat: -1, err: errRoomClosed}
	case cmdReady:
		m.res <- readyResult{err: errRoomClosed}
	case cmdLeave:
		m.res <- errRoomClosed
	case cmdRoundSnap:
		m.res <- roundSnapResult{err: errRoomClosed}
	case cmdRoundView:
		m.res <- roundViewResult{}
	case cmdRoomSnapshot:
		m.res <- roomSnapshotResult{}
	case cmdDiscard:
		m.res <- actionResult{err: errRoomClosed}
	case cmdPong:
		m.res <- actionResult{err: errRoomClosed}
	case cmdChi:
		m.res <- actionResult{err: errRoomClosed}
	case cmdGang:
		m.res <- actionResult{err: errRoomClosed}
	case cmdHu:
		m.res <- actionResult{err: errRoomClosed}
	case cmdPass:
		m.res <- actionResult{err: errRoomClosed}
	case cmdAutoTimeout:
		m.res <- actionResult{err: errRoomClosed}
	case cmdOpeningAction:
		m.res <- actionResult{err: errRoomClosed}
	case cmdShutdownTimers:
		// 房间关闭分支已清理全部定时器，停机语义天然达成，直接确认。
		m.res <- struct{}{}
		// cmdMarkOffline、cmdCancelOffline、cmdOfflineTimeout 无 res，直接丢弃。
	}
}

// submitJoin 向房间 actor 提交加入请求并同步等待结果（ctx 可取消防悬挂）。
func (a *Actor) SubmitJoin(ctx context.Context, userID string) (int, error) {
	if a == nil {
		return -1, fmt.Errorf("nil actor")
	}
	a.submitMu.Lock()
	if a.closed.Load() {
		a.submitMu.Unlock()
		return -1, fmt.Errorf("room closed")
	}
	res := make(chan joinResult, 1)
	cmd := cmdJoin{userID: userID, res: res}
	select {
	case a.ch <- cmd:
	default:
		a.submitMu.Unlock()
		return -1, ErrRateLimited
	case <-ctx.Done():
		a.submitMu.Unlock()
		return -1, ctx.Err()
	}
	a.submitMu.Unlock()
	select {
	case jr := <-res:
		return jr.seat, jr.err
	case <-ctx.Done():
		return -1, ctx.Err()
	}
}

// submitReady 向房间 actor 提交准备请求并同步等待结果。
func (a *Actor) SubmitReady(ctx context.Context, userID string) ([]Notification, error) {
	if a == nil {
		return nil, fmt.Errorf("nil actor")
	}
	a.submitMu.Lock()
	if a.closed.Load() {
		a.submitMu.Unlock()
		return nil, fmt.Errorf("room closed")
	}
	res := make(chan readyResult, 1)
	cmd := cmdReady{userID: userID, res: res}
	if cap(a.ch) == 0 {
		select {
		case a.ch <- cmd:
		case <-ctx.Done():
			a.submitMu.Unlock()
			return nil, ctx.Err()
		}
	} else {
		select {
		case a.ch <- cmd:
		default:
			a.submitMu.Unlock()
			return nil, ErrRateLimited
		case <-ctx.Done():
			a.submitMu.Unlock()
			return nil, ctx.Err()
		}
	}
	a.submitMu.Unlock()
	select {
	case rr := <-res:
		return rr.notifications, rr.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (a *Actor) SubmitLeave(ctx context.Context, userID string) error {
	if a == nil {
		return fmt.Errorf("nil actor")
	}
	a.submitMu.Lock()
	if a.closed.Load() {
		a.submitMu.Unlock()
		return fmt.Errorf("room closed")
	}
	res := make(chan error, 1)
	cmd := cmdLeave{userID: userID, res: res}
	select {
	case a.ch <- cmd:
	default:
		a.submitMu.Unlock()
		return ErrRateLimited
	case <-ctx.Done():
		a.submitMu.Unlock()
		return ctx.Err()
	}
	a.submitMu.Unlock()
	select {
	case err := <-res:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (a *Actor) SubmitDiscard(ctx context.Context, userID, tile string, tok *PhaseToken) ([]Notification, error) {
	return a.SubmitAction(ctx, cmdDiscard{userID: userID, tile: tile, phaseTok: tok, ctx: ctx, res: make(chan actionResult, 1)})
}

func (a *Actor) SubmitPong(ctx context.Context, userID string, tok *PhaseToken) ([]Notification, error) {
	return a.SubmitAction(ctx, cmdPong{userID: userID, phaseTok: tok, ctx: ctx, res: make(chan actionResult, 1)})
}

func (a *Actor) SubmitChi(ctx context.Context, userID string, tiles []string, tok *PhaseToken) ([]Notification, error) {
	return a.SubmitAction(ctx, cmdChi{userID: userID, tiles: tiles, phaseTok: tok, ctx: ctx, res: make(chan actionResult, 1)})
}

func (a *Actor) SubmitGang(ctx context.Context, userID, tile string, tok *PhaseToken) ([]Notification, error) {
	return a.SubmitAction(ctx, cmdGang{userID: userID, tile: tile, phaseTok: tok, ctx: ctx, res: make(chan actionResult, 1)})
}

func (a *Actor) SubmitHu(ctx context.Context, userID string, tok *PhaseToken) ([]Notification, error) {
	return a.SubmitAction(ctx, cmdHu{userID: userID, phaseTok: tok, ctx: ctx, res: make(chan actionResult, 1)})
}

func (a *Actor) SubmitPass(ctx context.Context, userID string, tok *PhaseToken) ([]Notification, error) {
	return a.SubmitAction(ctx, cmdPass{userID: userID, phaseTok: tok, ctx: ctx, res: make(chan actionResult, 1)})
}

// checkPhaseToken 是 actor 状态变更动作的统一前置校验；详见 ADR-0045。
// 调用时必须已位于 actor 单 goroutine 内（即在 run() 的 switch 中），
// 以保证 (rs.step, rs.phaseReason) 读取与后续 do* 写入是同一时刻的原子组合。
func (a *Actor) checkPhaseToken(tok *PhaseToken) error {
	if a == nil || a.round == nil {
		// 无进行中局面时放行：开局前客户端不持有合法令牌，
		// 由后续 do* 的 round-nil 检查负责拦截非法动作。
		return nil
	}
	return a.round.ValidatePhaseToken(tok)
}

func (a *Actor) SubmitAutoTimeout(ctx context.Context) ([]Notification, error) {
	return a.SubmitAction(ctx, cmdAutoTimeout{ctx: ctx, res: make(chan actionResult, 1)})
}

// Shutdown 停止本房间全部自驱动定时器并等待已在途的超时回调（含其持久化副作用）完成。
// 属于进程优雅停机契约的"停自驱动源"一步：必须在传输层排空之后、存储依赖关闭之前调用。
// 已关闭房间的定时器在关闭分支已清理，直接返回。
func (a *Actor) Shutdown(ctx context.Context) error {
	if a == nil {
		return nil
	}
	a.submitMu.Lock()
	if a.closed.Load() {
		a.submitMu.Unlock()
		return nil
	}
	res := make(chan struct{}, 1)
	select {
	case a.ch <- cmdShutdownTimers{res: res}:
	case <-ctx.Done():
		a.submitMu.Unlock()
		return ctx.Err()
	}
	a.submitMu.Unlock()
	select {
	case <-res:
	case <-ctx.Done():
		return ctx.Err()
	}
	if a.scheduler != nil {
		a.scheduler.waitInflight()
	}
	return nil
}

func (a *Actor) SubmitOpeningAction(ctx context.Context, userID, action string, tiles []string, direction, suit int32, params map[string]string, tok *PhaseToken) ([]Notification, error) {
	return a.SubmitAction(ctx, cmdOpeningAction{
		userID:    userID,
		action:    action,
		tiles:     append([]string(nil), tiles...),
		direction: direction,
		suit:      suit,
		params:    cloneMap(params),
		phaseTok:  tok,
		ctx:       ctx,
		res:       make(chan actionResult, 1),
	})
}

// logActionRejected 在 actor 单协程内统一记录动作拒绝日志，覆盖 token 漂移与状态不满足。
func (a *Actor) logActionRejected(ctx context.Context, userID, action string, tok *PhaseToken, err error) {
	if err == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if a != nil && a.Room != nil {
		ctx = logx.WithRoomID(ctx, a.Room.ID)
	}
	if userID != "" {
		ctx = logx.WithUserID(ctx, userID)
	}
	phaseStep := int64(0)
	phaseReason := "none"
	if a != nil && a.round != nil {
		phaseStep = int64(a.round.Step())
		phaseReason = a.round.PhaseReason().String()
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

func (a *Actor) SubmitRoundSnapJSON(ctx context.Context) ([]byte, error) {
	if a == nil {
		return nil, fmt.Errorf("nil actor")
	}
	a.submitMu.Lock()
	if a.closed.Load() {
		a.submitMu.Unlock()
		return nil, fmt.Errorf("room closed")
	}
	res := make(chan roundSnapResult, 1)
	cmd := cmdRoundSnap{res: res}
	if cap(a.ch) == 0 {
		select {
		case a.ch <- cmd:
		case <-ctx.Done():
			a.submitMu.Unlock()
			return nil, ctx.Err()
		}
	} else {
		select {
		case a.ch <- cmd:
		default:
			a.submitMu.Unlock()
			return nil, ErrRateLimited
		case <-ctx.Done():
			a.submitMu.Unlock()
			return nil, ctx.Err()
		}
	}
	a.submitMu.Unlock()
	select {
	case rr := <-res:
		return rr.data, rr.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (a *Actor) SubmitRoundView(ctx context.Context) (RoundView, bool, error) {
	if a == nil {
		return RoundView{}, false, fmt.Errorf("nil actor")
	}
	a.submitMu.Lock()
	if a.closed.Load() {
		a.submitMu.Unlock()
		return RoundView{}, false, fmt.Errorf("room closed")
	}
	res := make(chan roundViewResult, 1)
	cmd := cmdRoundView{res: res}
	select {
	case a.ch <- cmd:
	default:
		a.submitMu.Unlock()
		return RoundView{}, false, ErrRateLimited
	case <-ctx.Done():
		a.submitMu.Unlock()
		return RoundView{}, false, ctx.Err()
	}
	a.submitMu.Unlock()
	select {
	case rr := <-res:
		return rr.view, rr.ok, nil
	case <-ctx.Done():
		return RoundView{}, false, ctx.Err()
	}
}

func (a *Actor) SubmitRoomSnapshot(ctx context.Context) ([]string, string, [4]bool, error) {
	if a == nil {
		return nil, "", [4]bool{}, fmt.Errorf("nil actor")
	}
	a.submitMu.Lock()
	if a.closed.Load() {
		a.submitMu.Unlock()
		return nil, "", [4]bool{}, fmt.Errorf("room closed")
	}
	res := make(chan roomSnapshotResult, 1)
	cmd := cmdRoomSnapshot{res: res}
	select {
	case a.ch <- cmd:
	default:
		a.submitMu.Unlock()
		return nil, "", [4]bool{}, ErrRateLimited
	case <-ctx.Done():
		a.submitMu.Unlock()
		return nil, "", [4]bool{}, ctx.Err()
	}
	a.submitMu.Unlock()
	select {
	case rr := <-res:
		return rr.playerIDs, rr.fsmState, rr.ready, nil
	case <-ctx.Done():
		return nil, "", [4]bool{}, ctx.Err()
	}
}

func (a *Actor) SubmitAction(ctx context.Context, cmd any) ([]Notification, error) {
	if a == nil {
		return nil, fmt.Errorf("nil actor")
	}
	a.submitMu.Lock()
	if a.closed.Load() {
		a.submitMu.Unlock()
		return nil, fmt.Errorf("room closed")
	}
	if cap(a.ch) == 0 {
		select {
		case a.ch <- cmd:
		case <-ctx.Done():
			a.submitMu.Unlock()
			return nil, ctx.Err()
		}
	} else {
		select {
		case a.ch <- cmd:
		default:
			a.submitMu.Unlock()
			return nil, ErrRateLimited
		case <-ctx.Done():
			a.submitMu.Unlock()
			return nil, ctx.Err()
		}
	}
	a.submitMu.Unlock()
	// 等待响应（无锁），ctx 可取消防悬挂。
	var resCh chan actionResult
	switch c := cmd.(type) {
	case cmdDiscard:
		resCh = c.res
	case cmdPong:
		resCh = c.res
	case cmdChi:
		resCh = c.res
	case cmdGang:
		resCh = c.res
	case cmdHu:
		resCh = c.res
	case cmdPass:
		resCh = c.res
	case cmdAutoTimeout:
		resCh = c.res
	case cmdOpeningAction:
		resCh = c.res
	default:
		return nil, fmt.Errorf("unsupported action command")
	}
	select {
	case rr := <-resCh:
		return rr.notifications, rr.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// ensureOfflineTimer 在 actor goroutine 内启动（或重置）离线投降计时器。
// 定时器到期后直接向 ch 投递 cmdOfflineTimeout，保证全部投降逻辑在串行上下文内执行。
func (a *Actor) ensureOfflineTimer(userID string) {
	if a.draining {
		return
	}
	if a.offlineTimers == nil {
		a.offlineTimers = make(map[string]*time.Timer)
	}
	if a.offlineTimerGens == nil {
		a.offlineTimerGens = make(map[string]uint64)
	}
	if t, ok := a.offlineTimers[userID]; ok {
		t.Stop()
	}
	// 递增代号，作废旧定时器的回调（即使旧 AfterFunc 已触发，投递到 ch 的消息也会被代号检查拦截）。
	a.offlineTimerGens[userID]++
	gen := a.offlineTimerGens[userID]
	delay := a.offlineSurrenderAfter
	if delay <= 0 {
		delay = DefaultOfflineSurrenderAfter
	}
	a.offlineTimers[userID] = time.AfterFunc(delay, func() {
		// 只投递消息，不调用任何 submit*，不等待响应，无死锁风险。
		select {
		case a.ch <- cmdOfflineTimeout{userID: userID, gen: gen}:
		default:
			// mailbox 满或已关闭时丢弃；run() 关闭后不会再消费，可安全忽略。
		}
	})
}

// cancelOfflineTimer 取消玩家的离线投降计时器；无计时器时为空操作。
func (a *Actor) cancelOfflineTimer(userID string) {
	if t, ok := a.offlineTimers[userID]; ok {
		t.Stop()
		delete(a.offlineTimers, userID)
	}
	// 递增代号，作废已投递到 ch 但尚未被消费的 cmdOfflineTimeout。
	if a.offlineTimerGens != nil {
		a.offlineTimerGens[userID]++
	}
}

// submitMarkOffline 向 actor 邮箱投递离线事件（fire-and-forget，不等待响应）。
func (a *Actor) SubmitMarkOffline(userID string) {
	if a == nil || a.closed.Load() {
		return
	}
	a.submitMu.Lock()
	defer a.submitMu.Unlock()
	select {
	case a.ch <- cmdMarkOffline{userID: userID}:
	default:
		// 邮箱满时丢弃；投降计时器不会触发，玩家状态仍可通过后续 Leave 清理。
	}
}

// submitCancelOffline 向 actor 邮箱投递重连取消离线事件（fire-and-forget）。
func (a *Actor) SubmitCancelOffline(userID string) {
	if a == nil || a.closed.Load() {
		return
	}
	a.submitMu.Lock()
	defer a.submitMu.Unlock()
	select {
	case a.ch <- cmdCancelOffline{userID: userID}:
	default:
	}
}

func cloneMap(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
