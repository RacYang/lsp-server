// Package rules 定义可插拔麻将规则接口与注册表；具体变体放在子目录中实现。
package rules

import (
	"context"
	"fmt"
	"sync"

	"racoo.cn/lsp/internal/mahjong/fan"
	"racoo.cn/lsp/internal/mahjong/hand"
	"racoo.cn/lsp/internal/mahjong/hu"
	"racoo.cn/lsp/internal/mahjong/tile"
	"racoo.cn/lsp/internal/mahjong/wall"
)

// HuSource 表示和牌来源，供规则实现区分自摸、点炮与抢杠等场况。
type HuSource string

const (
	HuSourceUnspecified HuSource = ""
	HuSourceTsumo       HuSource = "tsumo"
	HuSourceDiscard     HuSource = "discard"
	HuSourceQiangGang   HuSource = "qiang_gang"
	HuSourceBuGang      HuSource = "bu_gang"
)

// GangKind 表示杠牌类型，供结算阶段计算刮风下雨与退税。
type GangKind string

const (
	GangKindUnspecified GangKind = ""
	GangKindMing        GangKind = "ming"
	GangKindAn          GangKind = "an"
	GangKindBu          GangKind = "bu"
)

// GangRecord 记录一笔杠牌流水，后续用于抢杠、退税与责任方判定。
type GangRecord struct {
	Seat            int
	Kind            GangKind
	Tile            tile.Tile
	FromSeat        int
	ResponsibleSeat int
	Step            int
}

// HuContext 为和牌判定上下文；规则实现可按需消费场况字段。
type HuContext struct {
	Source          HuSource
	PendingTile     tile.Tile
	Que             []tile.Suit
	Discarder       int
	IsHaiDi         bool
	IsGangShangHua  bool
	ResponsibleSeat int
	GangHistory     []GangRecord
	WallRemaining   int
}

// HuResult 保存和牌后的 14 张计数快照，供计分使用。
type HuResult struct {
	Win hu.Counts
}

// ScoreContext 为计分上下文；Phase 5 规则 PR 会逐步消费这些字段。
type ScoreContext struct {
	SeatGenTiles    [][]tile.Tile
	GangRecords     []GangRecord
	IsTsumo         bool
	IsHaiDi         bool
	IsGangShangHua  bool
	IsGangShangPao  bool
	Que             []tile.Suit
	ResponsibleSeat int
	WallRemaining   int
}

// GameState 描述血战到底结束条件所需的最小信息。
type GameState struct {
	// WallRemaining 牌墙剩余张数（摸牌堆）。
	WallRemaining int
	// HuedPlayers 已经和牌的人数。
	HuedPlayers int
}

// Rule 为玩法变体接口；房间层只应依赖本接口而非具体实现包。
type Rule interface {
	ID() string
	Name() string
	BuildWall(ctx context.Context, seed int64) *wall.Wall
	CheckHu(h *hand.Hand, target tile.Tile, hc HuContext) (HuResult, bool)
	ScoreFans(result HuResult, sc ScoreContext) fan.Breakdown
	GameOver(state GameState) bool
}

var (
	regMu sync.RWMutex
	reg   = map[string]Rule{}
)

// Register 注册规则实现；重复 ID 将 panic，避免静默覆盖。
func Register(r Rule) {
	if r == nil {
		panic("nil rule")
	}
	id := r.ID()
	if id == "" {
		panic("empty rule id")
	}
	regMu.Lock()
	defer regMu.Unlock()
	if _, ok := reg[id]; ok {
		panic(fmt.Sprintf("duplicate rule id: %s", id))
	}
	reg[id] = r
}

// MustGet 按 ID 获取规则；不存在则 panic（装配期错误应尽早暴露）。
func MustGet(id string) Rule {
	regMu.RLock()
	defer regMu.RUnlock()
	r, ok := reg[id]
	if !ok {
		panic(fmt.Sprintf("unknown rule id: %s", id))
	}
	return r
}
