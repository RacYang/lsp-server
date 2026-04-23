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

func TestRegisterAndMustGet(t *testing.T) {
	id := fmt.Sprintf("fake_rule_%s", t.Name())
	Register(&fakeRule{id: id})
	r := MustGet(id)
	if r.ID() != id {
		t.Fatalf("unexpected id %s", r.ID())
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
