package rules

import (
	"context"
	"fmt"
	"testing"

	"racoo.cn/lsp/internal/mahjong/fan"
	"racoo.cn/lsp/internal/mahjong/hand"
	"racoo.cn/lsp/internal/mahjong/tile"
	"racoo.cn/lsp/internal/mahjong/wall"
)

type fakeRule struct {
	id string
}

func (f *fakeRule) ID() string { return f.id }

func (f *fakeRule) Name() string { return "假规则用于注册表测试" }

func (f *fakeRule) BuildWall(ctx context.Context, seed int64) *wall.Wall {
	_ = ctx
	_ = seed
	return wall.NewFull108()
}

func (f *fakeRule) CheckHu(h *hand.Hand, target tile.Tile, hc HuContext) (HuResult, bool) {
	_ = h
	_ = target
	_ = hc
	return HuResult{}, false
}

func (f *fakeRule) ScoreFans(result HuResult, sc ScoreContext) fan.Breakdown {
	_ = result
	_ = sc
	return fan.Breakdown{}
}

func (f *fakeRule) GameOver(state GameState) bool {
	return state.WallRemaining == 0
}

type capabilityRule struct {
	fakeRule
	caps CapabilitySet
}

func (c *capabilityRule) Capabilities() CapabilitySet {
	return c.caps
}

func TestRegisterAndMustGet(t *testing.T) {
	id := fmt.Sprintf("fake_rule_%s", t.Name())
	Register(&fakeRule{id: id})
	r := MustGet(id)
	if r.ID() != id {
		t.Fatalf("unexpected id %s", r.ID())
	}
}

func TestListReturnsSortedSnapshot(t *testing.T) {
	prefix := fmt.Sprintf("list_rule_%s_", t.Name())
	Register(&fakeRule{id: prefix + "b"})
	Register(&fakeRule{id: prefix + "a"})

	got := List()
	ids := make([]string, 0, len(got))
	for _, r := range got {
		ids = append(ids, r.ID())
	}
	posA, posB := -1, -1
	for i, id := range ids {
		switch id {
		case prefix + "a":
			posA = i
		case prefix + "b":
			posB = i
		}
	}
	if posA < 0 || posB < 0 || posA > posB {
		t.Fatalf("List must contain a sorted snapshot, ids=%v", ids)
	}
	got[0] = &fakeRule{id: "mutated"}
	if MustGet(prefix+"a").ID() != prefix+"a" {
		t.Fatal("mutating List result should not affect registry")
	}
}

func TestMustGetPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	_ = MustGet("no_such_rule_ever_12345")
}

func TestDuplicateRegisterPanics(t *testing.T) {
	id := fmt.Sprintf("dup_rule_%s", t.Name())
	Register(&fakeRule{id: id})
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on duplicate register")
		}
	}()
	Register(&fakeRule{id: id})
}

func TestDefaultCapabilitiesProvideNoEatingClaimPolicy(t *testing.T) {
	r := &fakeRule{id: "capability_default"}
	caps := CapabilitiesOf(r)
	if caps.Claims == nil {
		t.Fatal("expected default claim policy")
	}
	target := tile.Must(tile.SuitCharacters, 3)
	actions := caps.Claims.Candidates(ClaimContext{
		Seat:       1,
		SourceSeat: 0,
		Tile:       target,
		Hand:       hand.FromTiles([]tile.Tile{target, target, target}),
		CheckHu: func(*hand.Hand, tile.Tile, HuContext) (HuResult, bool) {
			return HuResult{}, true
		},
	})
	got := make([]ActionName, 0, len(actions))
	for _, action := range actions {
		got = append(got, action.Name)
	}
	want := []ActionName{ActionHu, ActionGang, ActionPong}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("unexpected actions got=%v want=%v", got, want)
	}
}

func TestCapabilitiesOfProviderFillsMissingClaimPolicy(t *testing.T) {
	r := &capabilityRule{
		fakeRule: fakeRule{id: "capability_provider"},
		caps: CapabilitySet{
			Metadata: RuleMetadata{DisplayName: "能力测试"},
			Opening:  StaticOpeningFlow{"deal"},
		},
	}
	caps := CapabilitiesOf(r)
	if caps.Metadata.DisplayName != "能力测试" {
		t.Fatalf("metadata mismatch: %+v", caps.Metadata)
	}
	if caps.Claims == nil {
		t.Fatal("expected fallback claim policy")
	}
	if got := caps.Opening.Steps(); fmt.Sprint(got) != "[deal]" {
		t.Fatalf("unexpected opening steps: %v", got)
	}
}

func TestFeatureSetReturnsCopy(t *testing.T) {
	features := FeatureSet{"a", "b"}
	got := features.FeatureFlags()
	got[0] = "changed"
	if features[0] != "a" {
		t.Fatalf("FeatureFlags must return copy: %v", features)
	}
}

func TestNoEatingClaimPolicySkipsInvalidContexts(t *testing.T) {
	policy := NoEatingClaimPolicy{}
	target := tile.Must(tile.SuitCharacters, 3)
	cases := []ClaimContext{
		{Seat: 1, SourceSeat: 1, Tile: target, Hand: hand.FromTiles([]tile.Tile{target, target})},
		{Seat: 1, SourceSeat: 0, Tile: 0, Hand: hand.FromTiles([]tile.Tile{target, target})},
		{Seat: 1, SourceSeat: 0, Tile: target, Hand: nil},
		{Seat: 1, SourceSeat: 0, Tile: target, Hand: hand.FromTiles([]tile.Tile{target, target}), Hued: true},
	}
	for i, tc := range cases {
		if got := policy.Candidates(tc); len(got) != 0 {
			t.Fatalf("case %d got candidates: %v", i, got)
		}
	}
}

func TestNoEatingClaimPolicyQiangGangOnlyAllowsHu(t *testing.T) {
	target := tile.Must(tile.SuitDots, 5)
	actions := NoEatingClaimPolicy{}.Candidates(ClaimContext{
		Seat:            2,
		SourceSeat:      0,
		Tile:            target,
		Hand:            hand.FromTiles([]tile.Tile{target, target, target}),
		QiangGangWindow: true,
		CheckHu: func(*hand.Hand, tile.Tile, HuContext) (HuResult, bool) {
			return HuResult{}, true
		},
	})
	if len(actions) != 1 || actions[0].Name != ActionHu || actions[0].ChoiceAction != "qiang_gang_choice" {
		t.Fatalf("unexpected qiang gang actions: %+v", actions)
	}
}
