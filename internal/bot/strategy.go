package bot

import (
	"errors"
	"math/rand"
	"sort"
	"time"

	"racoo.cn/lsp/internal/mahjong/analysis"
	"racoo.cn/lsp/internal/mahjong/hand"
	"racoo.cn/lsp/internal/mahjong/tile"
)

// Difficulty 表示内置规则 AI 难度。
type Difficulty string

const (
	DifficultyEasy   Difficulty = "easy"
	DifficultyNormal Difficulty = "normal"
	DifficultyHard   Difficulty = "hard"
)

// RuleStrategyConfig 定义内置规则策略参数。
type RuleStrategyConfig struct {
	Difficulty Difficulty
	Rand       *rand.Rand
}

// NewRuleStrategy 创建内置规则策略。
func NewRuleStrategy(cfg RuleStrategyConfig) Strategy {
	if cfg.Difficulty == "" {
		cfg.Difficulty = DifficultyNormal
	}
	if cfg.Rand == nil {
		cfg.Rand = rand.New(rand.NewSource(time.Now().UnixNano())) //nolint:gosec // 策略扰动不用于安全边界。
	}
	return &ruleStrategy{difficulty: cfg.Difficulty, rnd: cfg.Rand}
}

type ruleStrategy struct {
	difficulty Difficulty
	rnd        *rand.Rand
}

func (s *ruleStrategy) Decide(ctx Context, view BotView) (Action, error) {
	if err := ctx.Err(); err != nil {
		return Action{}, err
	}
	switch view.WaitingAction {
	case "exchange_three":
		return s.decideExchange(view)
	case "que_men":
		return s.decideQueMen(view)
	case "discard":
		return s.decideDiscard(view)
	case "claim_window":
		return s.decideClaim(view)
	case "tsumo_window":
		return s.decideTsumo(view)
	default:
		return Action{Kind: ActionNone}, nil
	}
}

func (s *ruleStrategy) decideExchange(view BotView) (Action, error) {
	ts := parseTiles(view.HandTiles)
	if len(ts) < 3 {
		return Action{}, errors.New("手牌不足，无法换三张")
	}
	que := analysis.BestQueSuit(ts)
	chosen := analysis.BestExchangeThree(ts, que)
	if len(chosen) < 3 {
		return Action{}, errors.New("换三张候选不足")
	}
	return Action{Kind: ActionExchangeThree, Tiles: tilesToStrings(chosen[:3]), Suit: 3, Reason: "选择同花色弱张换三张"}, nil
}

func (s *ruleStrategy) decideQueMen(view BotView) (Action, error) {
	ts := parseTiles(view.HandTiles)
	return Action{Kind: ActionQueMen, Suit: int32(analysis.BestQueSuit(ts)), Reason: "选择最弱花色定缺"}, nil
}

func (s *ruleStrategy) decideDiscard(view BotView) (Action, error) {
	ts := parseTiles(view.HandTiles)
	if len(ts) == 0 {
		return Action{}, errors.New("无手牌可出")
	}
	que := ownQue(view)
	if containsAction(view.AvailableAction, "gang") && s.difficulty == DifficultyHard {
		if t, ok := selfGangTile(ts); ok {
			return Action{Kind: ActionGang, Tile: t.String(), Reason: "hard 档自杠"}, nil
		}
	}
	if s.difficulty == DifficultyEasy {
		return Action{Kind: ActionDiscard, Tile: easyDiscard(ts, que).String(), Reason: "easy 弱启发式出牌"}, nil
	}
	pub := publicInfo(view)
	options := analysis.DiscardOptions(ts, que, pub)
	if len(options) == 0 {
		return Action{Kind: ActionDiscard, Tile: ts[0].String(), Reason: "兜底出第一张"}, nil
	}
	best := chooseWithJitter(options, s.rnd)
	return Action{Kind: ActionDiscard, Tile: best.Tile.String(), Reason: "向听与安全度综合出牌"}, nil
}

func (s *ruleStrategy) decideClaim(view BotView) (Action, error) {
	if containsAction(view.AvailableAction, "hu") {
		return Action{Kind: ActionHu, Reason: "可胡必胡"}, nil
	}
	if s.difficulty == DifficultyEasy {
		return Action{Kind: ActionPass, Reason: "easy 档保守过"}, nil
	}
	if containsAction(view.AvailableAction, "gang") && view.PendingTile != "" && s.difficulty == DifficultyHard {
		return Action{Kind: ActionGang, Tile: view.PendingTile, Reason: "hard 档抢杠/明杠"}, nil
	}
	if containsAction(view.AvailableAction, "pong") && view.PendingTile != "" && s.difficulty != DifficultyEasy {
		t, err := tile.Parse(view.PendingTile)
		if err == nil && ownQue(view) != t.Suit() {
			return Action{Kind: ActionPong, Reason: "碰非缺门牌提速"}, nil
		}
	}
	return Action{Kind: ActionPass, Reason: "抢答窗口保守过"}, nil
}

func (s *ruleStrategy) decideTsumo(view BotView) (Action, error) {
	if containsAction(view.AvailableAction, "hu") {
		return Action{Kind: ActionHu, Reason: "自摸可胡必胡"}, nil
	}
	return Action{Kind: ActionPass, Reason: "无胡则过"}, nil
}

func publicInfo(view BotView) analysis.PublicInfo {
	return analysis.PublicInfo{
		DiscardsBySeat: view.DiscardsBySeat,
		MeldsBySeat:    view.MeldsBySeat,
		DrawnBySeat:    view.DrawnBySeat,
		QueBySeat:      view.QueBySeat[:],
		SelfSeat:       view.SeatIndex,
	}
}

func ownQue(view BotView) tile.Suit {
	if view.SeatIndex >= 0 && int(view.SeatIndex) < len(view.QueBySeat) {
		q := view.QueBySeat[view.SeatIndex]
		if q >= 0 && q <= 2 {
			return tile.Suit(q)
		}
	}
	return analysis.BestQueSuit(parseTiles(view.HandTiles))
}

func easyDiscard(ts []tile.Tile, que tile.Suit) tile.Tile {
	sort.Slice(ts, func(i, j int) bool {
		if ts[i].Suit() != ts[j].Suit() {
			return ts[i].Suit() < ts[j].Suit()
		}
		return ts[i].Rank() > ts[j].Rank()
	})
	for _, t := range ts {
		if t.Suit() == que {
			return t
		}
	}
	return ts[0]
}

func chooseWithJitter(options []analysis.DiscardOption, rnd *rand.Rand) analysis.DiscardOption {
	if len(options) <= 1 {
		return options[0]
	}
	top := []analysis.DiscardOption{options[0]}
	for _, opt := range options[1:] {
		if opt.Shanten != options[0].Shanten || opt.AdvanceKinds != options[0].AdvanceKinds || opt.AdvanceLeft != options[0].AdvanceLeft || opt.QueSuit != options[0].QueSuit {
			break
		}
		top = append(top, opt)
	}
	return top[rnd.Intn(len(top))]
}

func selfGangTile(ts []tile.Tile) (tile.Tile, bool) {
	counts := hand.FromTiles(ts).Counts()
	for idx, n := range counts {
		if n >= 4 {
			t, err := tile.FromIndex(idx)
			if err == nil {
				return t, true
			}
		}
	}
	return 0, false
}

func containsAction(actions []string, target string) bool {
	for _, action := range actions {
		if action == target {
			return true
		}
	}
	return false
}

func parseTiles(raws []string) []tile.Tile {
	out := make([]tile.Tile, 0, len(raws))
	for _, raw := range raws {
		t, err := tile.Parse(raw)
		if err == nil {
			out = append(out, t)
		}
	}
	return out
}

func tilesToStrings(ts []tile.Tile) []string {
	out := make([]string, 0, len(ts))
	for _, t := range ts {
		out = append(out, t.String())
	}
	return out
}
