package app

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"racoo.cn/lsp/internal/bot"
	roomsvc "racoo.cn/lsp/internal/service/room"
	"racoo.cn/lsp/pkg/logx"
)

// BotUserIDPrefix 是 lobby 给占位机器人分配的 user_id 前缀，约定见 ADR-0037。
// supervisor 直接按这个前缀识别 bot 座位，不需要额外协议字段。
const BotUserIDPrefix = "bot:"

// IsBotUserID 判断 user_id 是否为 lobby 分配的机器人座位。
func IsBotUserID(userID string) bool {
	return strings.HasPrefix(userID, BotUserIDPrefix)
}

// BotSupervisor 在 room 服务进程内代占位 bot 出牌：每当 actor 处理完一条命令，
// supervisor 异步检查是否有 bot 座位需要响应，依次走策略决策并提交回 Service。
//
// ADR-0037 描述的"占座 + 后续补 supervisor"的"后续"在此落地。supervisor 借助 Service
// 的公共入口（RoundView/PlayerIDs/Discard/...）观察并推进，因此被放在 app 层而不是 service 层，
// 避免麻将/网络/AI 三类耦合都集中到 service 包。
type BotSupervisor struct {
	svc      *roomsvc.Service
	strategy bot.Strategy

	mu      sync.Mutex
	pending map[string]*atomic.Int32

	maxIterations  int
	tickTimeout    time.Duration
	humanRoomDelay time.Duration
	postSubmit     func(roomID, userID string, action bot.Action) // 测试钩子；nil 时为生产路径。
}

// NewBotSupervisor 创建带默认参数的 supervisor，调用方需通过 Service.SetAfterCmdHook 把它接入。
func NewBotSupervisor(svc *roomsvc.Service) *BotSupervisor {
	return &BotSupervisor{
		svc:      svc,
		strategy: bot.NewRuleStrategy(bot.RuleStrategyConfig{Difficulty: bot.DifficultyNormal}),
		pending:  make(map[string]*atomic.Int32),
		// 一局完整对战通常 60~120 步，再加并发 exchange/quemen 各 4 次，
		// 单次 tick 上限设为 256 既能容纳一局，也能在策略 bug 时及时报警。
		maxIterations: 256,
		tickTimeout:   2 * time.Second,
		// 有真人在桌时，机器人出牌稍作停顿，让客户端有时间展示轮转和倒计时。
		humanRoomDelay: 650 * time.Millisecond,
	}
}

// SetStrategy 替换策略，主要供测试注入确定性策略。
func (b *BotSupervisor) SetStrategy(s bot.Strategy) {
	if b == nil || s == nil {
		return
	}
	b.strategy = s
}

// SetMaxIterations 限制单次 tick 最多让 bot 提交多少次动作，防止策略 bug 导致死循环。
func (b *BotSupervisor) SetMaxIterations(n int) {
	if b == nil || n <= 0 {
		return
	}
	b.maxIterations = n
}

// AfterCmd 是要挂到 Service.SetAfterCmdHook 的回调。
// 它必须保持非阻塞：内部只调度一次 tick goroutine，actor 主循环立即返回。
func (b *BotSupervisor) AfterCmd(roomID string) {
	if b == nil || roomID == "" {
		return
	}
	gate := b.gateForRoom(roomID)
	// 先把"待处理事件"计数 +1；如果当前没有 tick 在跑，由本次启动；否则交给运行中的 tick 处理。
	if gate.Add(1) != 1 {
		return
	}
	go b.tickRoom(roomID, gate)
}

func (b *BotSupervisor) gateForRoom(roomID string) *atomic.Int32 {
	b.mu.Lock()
	defer b.mu.Unlock()
	if g, ok := b.pending[roomID]; ok {
		return g
	}
	g := &atomic.Int32{}
	b.pending[roomID] = g
	return g
}

func (b *BotSupervisor) clearGateIfDrained(roomID string, gate *atomic.Int32) {
	if gate.Load() != 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if g, ok := b.pending[roomID]; ok && g == gate && g.Load() == 0 {
		delete(b.pending, roomID)
	}
}

// tickRoom 反复处理直到一次完整迭代下来 bot 都没有可执行动作（或达到 maxIterations）。
// 每次迭代都重新拉 RoundView，避免在 actor 串行处理过程中错过状态变更。
func (b *BotSupervisor) tickRoom(roomID string, gate *atomic.Int32) {
	defer func() {
		// 把当前批次的"已处理"计数清空；如果期间又涨了就再起一轮。
		for {
			cur := gate.Load()
			if cur == 0 {
				break
			}
			if gate.CompareAndSwap(cur, 0) {
				break
			}
		}
		b.clearGateIfDrained(roomID, gate)
	}()

	logCtx := logx.WithRoomID(context.Background(), roomID)
	for i := 0; i < b.maxIterations; i++ {
		acted, err := b.processOnce(roomID)
		if err != nil {
			logx.Warn(logCtx, "占座机器人调度循环执行失败，跳过本轮", "err", err.Error())
			return
		}
		if !acted {
			return
		}
	}
	logx.Warn(logCtx, "占座机器人调度循环达到单轮最大迭代次数，已主动中止以防失控", "max", b.maxIterations)
}

// processOnce 让所有可行动的 bot 座位提交一次动作；只要至少一个 bot 真的提交，就返回 true。
// 注意：exchange_three / que_men 阶段四家可并发提交，所以会循环遍历所有 bot 座位；
// discard / claim / tsumo 单座位生效，"acted" 通常只来自 ActingSeat 那个 bot。
func (b *BotSupervisor) processOnce(roomID string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.tickTimeout)
	defer cancel()
	view, ok, err := b.svc.RoundView(ctx, roomID)
	if err != nil || !ok || view.Closed {
		return false, err
	}
	playerIDs, ok := b.svc.PlayerIDs(roomID)
	if !ok {
		return false, nil
	}

	acted := false
	for seat := int32(0); seat < 4; seat++ {
		userID := playerIDs[seat]
		if !IsBotUserID(userID) {
			continue
		}
		seatCtx := logx.WithUserID(logx.WithRoomID(ctx, roomID), userID)
		didAct, err := b.tickBotSeat(seatCtx, roomID, userID, seat, view, roomHasHuman(playerIDs))
		if err != nil {
			logx.Warn(seatCtx, "占座机器人单座位决策失败，本轮跳过该座位", "seat", seat, "err", err.Error())
			continue
		}
		if didAct {
			acted = true
			// exchange / que_men 之外的等待态，状态机一旦推进，view 已失效，
			// 让外层 tickRoom 拉新的 RoundView 再决策。
			if view.WaitingAction != "exchange_three" && view.WaitingAction != "que_men" {
				return true, nil
			}
		}
	}
	return acted, nil
}

func (b *BotSupervisor) tickBotSeat(ctx context.Context, roomID, userID string, seat int32, view roomsvc.RoundView, humanRoom bool) (bool, error) {
	if !botShouldAct(seat, view) {
		return false, nil
	}
	if !IsBotUserID(userID) {
		return false, nil
	}
	if int(seat) >= len(view.PlayerIDs) || view.PlayerIDs[seat] != userID {
		return false, nil
	}
	bv := buildBotView(userID, roomID, seat, view)
	action, err := b.strategy.Decide(ctx, bv)
	if err != nil {
		return false, err
	}
	if action.Kind == bot.ActionNone || action.Kind == "" {
		return false, nil
	}
	if humanRoom && shouldDelayBotAction(view.WaitingAction) && b.humanRoomDelay > 0 {
		timer := time.NewTimer(b.humanRoomDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return false, ctx.Err()
		case <-timer.C:
		}
	}
	if err := b.submit(ctx, roomID, userID, action); err != nil {
		return false, err
	}
	if b.postSubmit != nil {
		b.postSubmit(roomID, userID, action)
	}
	return true, nil
}

func roomHasHuman(playerIDs [4]string) bool {
	for _, userID := range playerIDs {
		if userID != "" && !IsBotUserID(userID) {
			return true
		}
	}
	return false
}

func shouldDelayBotAction(waitingAction string) bool {
	switch waitingAction {
	case "discard", "claim_window", "tsumo_window":
		return true
	default:
		return false
	}
}

// botShouldAct 在不构造 BotView 的前提下快速过滤"轮不到我"的状态。
func botShouldAct(seat int32, view roomsvc.RoundView) bool {
	switch view.WaitingAction {
	case "exchange_three":
		if int(seat) >= len(view.ExchangeSubmitted) {
			return true
		}
		return !view.ExchangeSubmitted[seat]
	case "que_men":
		if int(seat) >= len(view.QueSubmitted) {
			return true
		}
		return !view.QueSubmitted[seat]
	case "discard", "tsumo_window":
		return view.ActingSeat == seat
	case "claim_window":
		for _, c := range view.ClaimCandidates {
			if c.Seat == seat && len(c.Actions) > 0 {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// buildBotView 把 RoundView 投影到指定 bot 座位的 BotView，供策略决策。
func buildBotView(userID, roomID string, seat int32, view roomsvc.RoundView) bot.BotView {
	bv := bot.BotView{
		UserID:        userID,
		RoomID:        roomID,
		SeatIndex:     seat,
		WaitingAction: view.WaitingAction,
		ActingSeat:    view.ActingSeat,
		PendingTile:   view.PendingTile,
		UpdatedAt:     time.Now(),
	}
	bv.AvailableAction = botAvailableForSeat(seat, view)
	if int(seat) < len(view.HandsBySeat) {
		bv.HandTiles = append([]string(nil), view.HandsBySeat[seat]...)
	}
	bv.DiscardsBySeat = cloneStringMatrix4(view.DiscardsBySeat)
	bv.MeldsBySeat = cloneStringMatrix4(view.MeldsBySeat)
	// RoundView 暂未追踪每家摸牌历史；用空切片兜底（仅影响 RemainingByPublic 的精度，不阻塞决策）。
	bv.DrawnBySeat = make([][]string, 4)
	for i := 0; i < 4 && i < len(view.QueBySeat); i++ {
		bv.QueBySeat[i] = view.QueBySeat[i]
	}
	if view.WaitingAction == "claim_window" {
		bv.ClaimCandidates = make(map[int32][]string, len(view.ClaimCandidates))
		for _, c := range view.ClaimCandidates {
			bv.ClaimCandidates[c.Seat] = append([]string(nil), c.Actions...)
		}
	}
	return bv
}

// botAvailableForSeat 返回该座位在当前等待态下"自己能做"的动作列表。
// claim_window 是按候选座位分发的，不能盲目套 view.AvailableActions。
func botAvailableForSeat(seat int32, view roomsvc.RoundView) []string {
	switch view.WaitingAction {
	case "claim_window":
		for _, c := range view.ClaimCandidates {
			if c.Seat == seat {
				return append([]string(nil), c.Actions...)
			}
		}
		return nil
	case "discard", "tsumo_window":
		if view.ActingSeat == seat {
			return append([]string(nil), view.AvailableActions...)
		}
		return nil
	default:
		return append([]string(nil), view.AvailableActions...)
	}
}

func (b *BotSupervisor) submit(ctx context.Context, roomID, userID string, action bot.Action) error {
	switch action.Kind {
	case bot.ActionExchangeThree:
		_, err := b.svc.ExchangeThree(ctx, roomID, userID, action.Tiles, action.Suit)
		return err
	case bot.ActionQueMen:
		_, err := b.svc.QueMen(ctx, roomID, userID, action.Suit)
		return err
	case bot.ActionDiscard:
		_, err := b.svc.Discard(ctx, roomID, userID, action.Tile)
		return err
	case bot.ActionPong:
		_, err := b.svc.Pong(ctx, roomID, userID)
		return err
	case bot.ActionGang:
		_, err := b.svc.Gang(ctx, roomID, userID, action.Tile)
		return err
	case bot.ActionHu:
		_, err := b.svc.Hu(ctx, roomID, userID)
		return err
	case bot.ActionPass:
		_, err := b.svc.Pass(ctx, roomID, userID)
		return err
	case bot.ActionReady:
		_, err := b.svc.Ready(ctx, roomID, userID)
		return err
	default:
		return nil
	}
}

func cloneStringMatrix4(in [][]string) [][]string {
	out := make([][]string, 4)
	for i := 0; i < len(out) && i < len(in); i++ {
		out[i] = append([]string(nil), in[i]...)
	}
	return out
}
