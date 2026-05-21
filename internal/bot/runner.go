package bot

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/bot/wsclient"
	"racoo.cn/lsp/internal/net/msgid"
	"racoo.cn/lsp/pkg/logx"
)

// Runner 管理单个机器人连接、状态与策略决策。
type Runner struct {
	cfg        RunnerConfig
	state      *BotState
	step       int64
	rejections int
	rnd        *rand.Rand
}

// NewRunner 创建单个机器人 runner。
func NewRunner(cfg RunnerConfig) *Runner {
	if cfg.Strategy == nil {
		cfg.Strategy = NewRuleStrategy(RuleStrategyConfig{Difficulty: DifficultyNormal})
	}
	return &Runner{
		cfg:   cfg,
		state: NewState(),
		rnd:   rand.New(rand.NewSource(time.Now().UnixNano())), //nolint:gosec // 仅用于思考延迟抖动。
	}
}

// Run 持续运行机器人，断线后按指数退避重连。
func (r *Runner) Run(ctx context.Context) error {
	backoff := time.Second
	for ctx.Err() == nil {
		err := r.runOnce(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		logx.Warn(ctx, "机器人连接结束，准备重连", "name", r.cfg.Name, "err", errString(err))
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
	return ctx.Err()
}

func (r *Runner) runOnce(ctx context.Context) error {
	client := wsclient.New(r.cfg.WSURL, r.cfg.Name, r.cfg.TokenFile, r.cfg.Origin, r.cfg.InsecureSkipVerify)
	if err := client.Connect(ctx); err != nil {
		return err
	}
	defer client.Close()
	loopCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- client.RunLoops(loopCtx) }()
	for {
		select {
		case <-ctx.Done():
			_ = r.sendLeave(context.Background(), client)
			return ctx.Err()
		case err := <-errCh:
			return err
		case env := <-client.Events():
			r.state.Apply(env)
			if err := r.afterEvent(ctx, client, env); err != nil {
				logx.Warn(ctx, "机器人处理事件失败", "name", r.cfg.Name, "err", err.Error())
				return err
			}
		}
	}
}

func (r *Runner) afterEvent(ctx context.Context, client *wsclient.Client, env *clientv1.Envelope) error {
	if login := env.GetLoginResp(); login != nil {
		if login.GetErrorCode() != clientv1.ErrorCode_ERROR_CODE_UNSPECIFIED {
			if staleLoginSession(login) {
				client.ClearToken()
			}
			return fmt.Errorf("登录失败: %s", login.GetErrorMessage())
		}
		return r.sendJoin(ctx, client)
	}
	if jr := env.GetJoinRoomResp(); jr != nil {
		if jr.GetErrorCode() != clientv1.ErrorCode_ERROR_CODE_UNSPECIFIED {
			return fmt.Errorf("入房失败: %s", jr.GetErrorMessage())
		}
		return r.sendReady(ctx, client)
	}
	if settlement := env.GetSettlement(); settlement != nil {
		_ = settlement
		r.sleepThink(ctx, 500*time.Millisecond, 1500*time.Millisecond)
		return r.sendReady(ctx, client)
	}
	if rejected(env) {
		r.rejections++
		return nil
	} else if actionResponse(env) {
		r.rejections = 0
		return nil
	}
	view := r.state.Snapshot()
	if view.Closed {
		return context.Canceled
	}
	if !shouldDecide(view) {
		return nil
	}
	return r.decideAndSend(ctx, client, view)
}

func staleLoginSession(login *clientv1.LoginResponse) bool {
	if login == nil {
		return false
	}
	return login.GetErrorCode() == clientv1.ErrorCode_ERROR_CODE_UNAUTHORIZED ||
		login.GetErrorCode() == clientv1.ErrorCode_ERROR_CODE_ROOM_NOT_FOUND ||
		strings.Contains(login.GetErrorMessage(), "room not found")
}

func (r *Runner) decideAndSend(ctx context.Context, client *wsclient.Client, view BotView) error {
	r.sleepForWaiting(ctx, view.WaitingAction)
	action, err := r.cfg.Strategy.Decide(ctx, view)
	if err != nil || action.Kind == ActionNone {
		return err
	}
	return r.sendAction(ctx, client, action)
}

func shouldDecide(view BotView) bool {
	if view.SeatIndex < 0 {
		return false
	}
	if isOpeningDecision(view) {
		return true
	}
	switch view.WaitingAction {
	case "discard", "tsumo_window":
		return view.ActingSeat == view.SeatIndex
	case "claim_window":
		actions := view.ClaimCandidates[view.SeatIndex]
		return len(actions) > 0 || view.ActingSeat == view.SeatIndex
	default:
		return false
	}
}

func isOpeningDecision(view BotView) bool {
	switch view.WaitingAction {
	case "", "none", "discard", "claim_window", "tsumo_window":
		return false
	default:
		return containsAction(view.AvailableAction, view.WaitingAction) && len(view.HandTiles) >= 3
	}
}

func (r *Runner) sendJoin(ctx context.Context, client *wsclient.Client) error {
	return client.Send(ctx, msgid.JoinRoomReq, &clientv1.Envelope{
		ReqId: newReqID("join"),
		Body:  &clientv1.Envelope_JoinRoomReq{JoinRoomReq: &clientv1.JoinRoomRequest{RoomId: r.cfg.RoomID}},
	})
}

func (r *Runner) sendReady(ctx context.Context, client *wsclient.Client) error {
	return client.Send(ctx, msgid.ReadyReq, &clientv1.Envelope{
		ReqId:          newReqID("ready"),
		IdempotencyKey: r.idempotencyKey(msgid.ReadyReq),
		Body:           &clientv1.Envelope_ReadyReq{ReadyReq: &clientv1.ReadyRequest{}},
	})
}

func (r *Runner) sendLeave(ctx context.Context, client *wsclient.Client) error {
	return client.Send(ctx, msgid.LeaveRoomReq, &clientv1.Envelope{
		ReqId:          newReqID("leave"),
		IdempotencyKey: r.idempotencyKey(msgid.LeaveRoomReq),
		Body:           &clientv1.Envelope_LeaveRoomReq{LeaveRoomReq: &clientv1.LeaveRoomRequest{}},
	})
}

func (r *Runner) sendAction(ctx context.Context, client *wsclient.Client, action Action) error {
	switch action.Kind {
	case ActionExchangeThree:
		r.state.RememberExchange(action.Tiles)
		return client.Send(ctx, msgid.OpeningActionReq, &clientv1.Envelope{
			ReqId:          newReqID("exchange"),
			IdempotencyKey: r.idempotencyKey(msgid.OpeningActionReq),
			Body: &clientv1.Envelope_OpeningActionReq{OpeningActionReq: &clientv1.OpeningActionRequest{
				Action: string(ActionExchangeThree),
				Tiles:  append([]string(nil), action.Tiles...),
			}},
		})
	case ActionQueMen:
		return client.Send(ctx, msgid.OpeningActionReq, &clientv1.Envelope{
			ReqId:          newReqID("que"),
			IdempotencyKey: r.idempotencyKey(msgid.OpeningActionReq),
			Body: &clientv1.Envelope_OpeningActionReq{OpeningActionReq: &clientv1.OpeningActionRequest{
				Action: string(ActionQueMen),
				Suit:   action.Suit,
			}},
		})
	case ActionDiscard:
		return client.Send(ctx, msgid.DiscardReq, &clientv1.Envelope{
			ReqId:          newReqID("discard"),
			IdempotencyKey: r.idempotencyKey(msgid.DiscardReq),
			Body:           &clientv1.Envelope_DiscardReq{DiscardReq: &clientv1.DiscardRequest{Tile: action.Tile}},
		})
	case ActionPong:
		return client.Send(ctx, msgid.PongReq, &clientv1.Envelope{
			ReqId:          newReqID("pong"),
			IdempotencyKey: r.idempotencyKey(msgid.PongReq),
			Body:           &clientv1.Envelope_PongReq{PongReq: &clientv1.PongRequest{}},
		})
	case ActionGang:
		return client.Send(ctx, msgid.GangReq, &clientv1.Envelope{
			ReqId:          newReqID("gang"),
			IdempotencyKey: r.idempotencyKey(msgid.GangReq),
			Body:           &clientv1.Envelope_GangReq{GangReq: &clientv1.GangRequest{Tile: action.Tile}},
		})
	case ActionHu:
		return client.Send(ctx, msgid.HuReq, &clientv1.Envelope{
			ReqId:          newReqID("hu"),
			IdempotencyKey: r.idempotencyKey(msgid.HuReq),
			Body:           &clientv1.Envelope_HuReq{HuReq: &clientv1.HuRequest{}},
		})
	case ActionPass:
		return client.Send(ctx, msgid.PassReq, &clientv1.Envelope{
			ReqId:          newReqID("pass"),
			IdempotencyKey: r.idempotencyKey(msgid.PassReq),
			Body:           &clientv1.Envelope_PassReq{PassReq: &clientv1.PassRequest{}},
		})
	default:
		return nil
	}
}

func (r *Runner) idempotencyKey(id uint16) string {
	r.step++
	userID := r.state.Snapshot().UserID
	if userID == "" {
		userID = r.cfg.Name
	}
	return fmt.Sprintf("%s:%d:%d", userID, id, r.step)
}

func (r *Runner) sleepForWaiting(ctx context.Context, waiting string) {
	switch waiting {
	case "claim_window", "tsumo_window":
		r.sleepThink(ctx, 700*time.Millisecond, 1600*time.Millisecond)
	case "discard":
		r.sleepThink(ctx, 900*time.Millisecond, 2400*time.Millisecond)
	default:
		r.sleepThink(ctx, time.Second, 2500*time.Millisecond)
	}
}

func (r *Runner) sleepThink(ctx context.Context, min, max time.Duration) {
	if r.cfg.ThinkMin > 0 || r.cfg.ThinkMax > 0 {
		min = r.cfg.ThinkMin
		max = r.cfg.ThinkMax
	}
	if min <= 0 && max <= 0 {
		return
	}
	if max < min {
		max = min
	}
	d := min
	if max > min {
		d += time.Duration(r.rnd.Int63n(int64(max - min)))
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

func rejected(env *clientv1.Envelope) bool {
	return responseCode(env) != clientv1.ErrorCode_ERROR_CODE_UNSPECIFIED && actionResponse(env)
}

func actionResponse(env *clientv1.Envelope) bool {
	switch env.GetBody().(type) {
	case *clientv1.Envelope_ReadyResp,
		*clientv1.Envelope_OpeningActionResp,
		*clientv1.Envelope_DiscardResp,
		*clientv1.Envelope_PongResp,
		*clientv1.Envelope_GangResp,
		*clientv1.Envelope_HuResp,
		*clientv1.Envelope_PassResp:
		return true
	default:
		return false
	}
}

func responseCode(env *clientv1.Envelope) clientv1.ErrorCode {
	switch body := env.GetBody().(type) {
	case *clientv1.Envelope_ReadyResp:
		return body.ReadyResp.GetErrorCode()
	case *clientv1.Envelope_OpeningActionResp:
		return body.OpeningActionResp.GetErrorCode()
	case *clientv1.Envelope_DiscardResp:
		return body.DiscardResp.GetErrorCode()
	case *clientv1.Envelope_PongResp:
		return body.PongResp.GetErrorCode()
	case *clientv1.Envelope_GangResp:
		return body.GangResp.GetErrorCode()
	case *clientv1.Envelope_HuResp:
		return body.HuResp.GetErrorCode()
	case *clientv1.Envelope_PassResp:
		return body.PassResp.GetErrorCode()
	default:
		return clientv1.ErrorCode_ERROR_CODE_UNSPECIFIED
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func newReqID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}
