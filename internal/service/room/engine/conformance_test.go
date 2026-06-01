package engine

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/mahjong/fan"
	"racoo.cn/lsp/internal/mahjong/hand"
	"racoo.cn/lsp/internal/mahjong/hu"
	"racoo.cn/lsp/internal/mahjong/rules"
	"racoo.cn/lsp/internal/mahjong/tile"
	"racoo.cn/lsp/internal/mahjong/wall"
)

const (
	conformanceFirstWinID    = "test_conformance_first_win"
	conformanceContinueID    = "test_conformance_continue_out"
	conformanceXueliuID      = "test_conformance_continue_in"
	conformanceMultiHuID     = "test_conformance_multi_hu"
	conformanceOpeningID     = "test_conformance_opening"
	conformanceOpeningAction = "custom_opening"
)

func init() {
	rules.Register(conformanceRule{id: conformanceFirstWinID, terminateWins: 1})
	rules.Register(conformanceRule{id: conformanceContinueID, terminateWins: 2})
	rules.Register(conformanceRule{id: conformanceXueliuID, terminateWins: 2, huedContinues: true})
	rules.Register(conformanceRule{id: conformanceMultiHuID, terminateWins: 2, claims: multiHuClaimPolicy{}})
	rules.Register(conformanceRule{id: conformanceOpeningID, terminateWins: 1, opening: customOpeningPolicy{}})
}

func TestRuleStrategyConformanceHuAftermathAndTermination(t *testing.T) {
	t.Parallel()

	immediate := conformanceRound(conformanceFirstWinID)
	notifs, err := NewEngine(conformanceFirstWinID).ApplyHu(context.Background(), immediate, 0)
	require.NoError(t, err)
	require.True(t, immediate.closed)
	require.True(t, containsSettlement(notifs))

	continueOut := conformanceRound(conformanceContinueID)
	notifs, err = NewEngine(conformanceContinueID).ApplyHu(context.Background(), continueOut, 0)
	require.NoError(t, err)
	require.False(t, continueOut.closed)
	require.False(t, containsSettlement(notifs))
	require.True(t, continueOut.isSeatOutAfterHu(0))
	require.Equal(t, Seat(1), continueOut.turn)

	xueliu := conformanceRound(conformanceXueliuID)
	notifs, err = NewEngine(conformanceXueliuID).ApplyHu(context.Background(), xueliu, 0)
	require.NoError(t, err)
	require.False(t, xueliu.closed)
	require.False(t, containsSettlement(notifs))
	require.False(t, xueliu.isSeatOutAfterHu(0))
}

func TestRuleStrategyConformanceMultiHuClaimWindow(t *testing.T) {
	t.Parallel()

	rs := conformanceRound(conformanceMultiHuID)
	rs.turn = 0
	rs.waitingTsumo = false
	rs.waitingDiscard = false
	rs.lastDiscard = tile.Must(tile.SuitCharacters, 1)
	rs.lastDiscardSeat = 0
	rs.openClaimWindow()
	require.Equal(t, Seat(1), rs.claimSeat())

	notifs, err := NewEngine(conformanceMultiHuID).ApplyHu(context.Background(), rs, 1)
	require.NoError(t, err)
	require.False(t, rs.closed)
	require.False(t, containsSettlement(notifs))
	require.Equal(t, Seat(2), rs.claimSeat())

	notifs, err = NewEngine(conformanceMultiHuID).ApplyHu(context.Background(), rs, 2)
	require.NoError(t, err)
	require.True(t, rs.closed)
	require.True(t, containsSettlement(notifs))
}

func TestRuleStrategyConformanceCustomOpeningProjection(t *testing.T) {
	t.Parallel()

	_, notifs, err := NewEngine(conformanceOpeningID).StartRound(context.Background(), "r-opening", [4]string{"u0", "u1", "u2", "u3"})
	require.NoError(t, err)
	require.NotEmpty(t, notifs)
	var found bool
	for _, notification := range notifs {
		if notification.Kind != KindAction {
			continue
		}
		env := decodeEnvelopeForTest(t, notification.Payload)
		if action := env.GetAction(); action != nil && action.GetAction() == conformanceOpeningAction {
			found = true
			break
		}
	}
	require.True(t, found)
}

func conformanceRound(ruleID string) *RoundState {
	rule := rules.MustGet(ruleID)
	caps := rules.CapabilitiesOf(rule)
	draw := tile.Must(tile.SuitCharacters, 1)
	rs := &RoundState{
		roomID:          "r-" + ruleID,
		ruleID:          ruleID,
		playerIDs:       [4]string{"u0", "u1", "u2", "u3"},
		rule:            rule,
		caps:            caps,
		wall:            wall.NewFromOrderedTiles([]tile.Tile{tile.Must(tile.SuitDots, 1), tile.Must(tile.SuitDots, 2), tile.Must(tile.SuitDots, 3), tile.Must(tile.SuitDots, 4)}),
		hands:           []*hand.Hand{hand.New(), hand.New(), hand.New(), hand.New()},
		ruleState:       newInitialRuleState(caps),
		discards:        make([][]tile.Tile, 4),
		flowers:         make([][]tile.Tile, 4),
		melds:           make([][]string, 4),
		surrendered:     make([]bool, 4),
		lastDiscardSeat: -1,
		turn:            0,
		waitingTsumo:    true,
		pendingDraw:     draw,
		currentDraw:     draw,
		winEvents:       make([]rules.WinEvent, 0, 2),
		scoreEvents:     make([]rules.ScoreEvent, 0, 8),
	}
	return rs
}

type conformanceRule struct {
	id            string
	terminateWins int
	huedContinues bool
	claims        rules.ClaimPolicy
	opening       rules.OpeningPolicy
}

func (r conformanceRule) ID() string   { return r.id }
func (r conformanceRule) Name() string { return "conformance " + r.id }
func (r conformanceRule) BuildWall(context.Context, int64) *wall.Wall {
	return wall.NewFromOrderedTiles(conformanceWallTiles())
}
func (r conformanceRule) CheckHu(h *hand.Hand, target tile.Tile, hc rules.HuContext) (rules.HuResult, bool) {
	_ = h
	_ = hc
	if target == 0 || target.IsFlower() {
		return rules.HuResult{}, false
	}
	var c hu.Counts
	c[target.Index()] = 2
	return rules.HuResult{Win: c, Closed: c}, true
}
func (r conformanceRule) Capabilities() rules.CapabilitySet {
	claimPolicy := r.claims
	if claimPolicy == nil {
		claimPolicy = rules.StandardClaimPolicy{}
	}
	return rules.CapabilitySet{
		Metadata:    rules.RuleMetadata{DisplayName: r.Name(), MaxHands: 4},
		TileSet:     r,
		Opening:     r.opening,
		Claims:      claimPolicy,
		SelfActions: rules.StandardSelfActionPolicy{},
		Win:         r,
		State:       rules.EmptyRuleStatePolicy{},
		StateView:   rules.EmptyRuleStatePolicy{},
		Turn:        conformanceTurn{huedContinues: r.huedContinues},
		Scoring:     conformanceScoring{},
		Settlement:  conformanceSettlement{},
		Termination: conformanceTermination{wins: r.terminateWins},
		Projection:  rules.FeatureSet{"conformance"},
	}
}

type conformanceTurn struct {
	huedContinues bool
}

func (conformanceTurn) FeatureFlags() []string { return []string{"conformance"} }
func (p conformanceTurn) HuedSeatContinues() bool {
	return p.huedContinues
}

type conformanceSettlement struct{}

func (conformanceSettlement) FeatureFlags() []string { return []string{"conformance"} }
func (conformanceSettlement) BuildSettlement(ctx rules.SettlementContext) rules.SettlementResult {
	return rules.SettlementResult{
		WinnerUserIDs: winnerUserIDs(ctx.PlayerIDs, seatsFromWinEvents(ctx.WinEvents)),
		SeatScores:    defaultSeatScores(ctx.PlayerIDs, ctx.ScoreEvents),
		DetailText:    fmt.Sprintf("%d wins", len(ctx.WinEvents)),
	}
}

type conformanceScoring struct{}

func (conformanceScoring) FeatureFlags() []string { return []string{"conformance"} }
func (conformanceScoring) ScoreWin(result rules.HuResult, sc rules.ScoreContext) (fan.Breakdown, []rules.ScoreEvent, bool) {
	var b fan.Breakdown
	b.Add(fan.Kind("conformance"), 1, "Conformance")
	events := make([]rules.ScoreEvent, 0, len(sc.ActiveSeats))
	for _, from := range sc.ActiveSeats {
		if from == sc.HuSeat {
			continue
		}
		events = append(events, rules.ScoreEvent{Reason: "conformance_hu", FromSeat: from, ToSeat: sc.HuSeat, Amount: 1, Step: sc.Step, WinnerSeat: sc.HuSeat, WinnerFan: 1, FanNames: []string{"Conformance"}})
	}
	_ = result
	return b, events, true
}
func (conformanceScoring) ScoreGang(sc rules.GangScoreContext) ([]rules.ScoreEvent, rules.GangRecord) {
	return nil, rules.GangRecord{Seat: sc.Seat, Kind: sc.Kind, Tile: sc.Tile, FromSeat: sc.FromSeat, ResponsibleSeat: sc.FromSeat, Step: sc.Step}
}

type conformanceTermination struct {
	wins int
}

func (conformanceTermination) FeatureFlags() []string { return []string{"conformance"} }
func (p conformanceTermination) GameOver(ctx rules.TerminationContext) bool {
	return len(ctx.WinEvents) >= p.wins || ctx.WallRemaining <= 0
}

type multiHuClaimPolicy struct{}

func (multiHuClaimPolicy) Candidates(ctx rules.ClaimContext) []rules.ClaimAction {
	if ctx.Seat == ctx.SourceSeat || ctx.Seat == 3 {
		return nil
	}
	return []rules.ClaimAction{{Name: rules.ActionHu, ChoiceAction: "hu_choice", Priority: 9}}
}

type customOpeningPolicy struct{}

func (customOpeningPolicy) Steps() []string { return []string{conformanceOpeningAction} }
func (customOpeningPolicy) InitialState() rules.RuleState {
	return rules.StaticOpeningFlow{conformanceOpeningAction}.InitialState()
}
func (customOpeningPolicy) CurrentStep(ctx rules.OpeningContext) (*rules.OpeningStep, bool) {
	return rules.StaticOpeningFlow{conformanceOpeningAction}.CurrentStep(ctx)
}
func (customOpeningPolicy) Apply(ctx rules.OpeningActionContext) (rules.OpeningResult, error) {
	return rules.StaticOpeningFlow{conformanceOpeningAction}.Apply(ctx)
}

func conformanceWallTiles() []tile.Tile {
	out := make([]tile.Tile, 0, 64)
	for i := 0; i < 64; i++ {
		out = append(out, tile.Must(tile.SuitCharacters, i%9+1))
	}
	return out
}

func seatsFromWinEvents(events []rules.WinEvent) []Seat {
	out := make([]Seat, 0, len(events))
	for _, event := range events {
		out = append(out, event.Seat)
	}
	return out
}

func decodeEnvelopeForTest(t *testing.T, payload []byte) *clientv1.Envelope {
	t.Helper()
	var env clientv1.Envelope
	require.NoError(t, proto.Unmarshal(payload, &env))
	return &env
}
