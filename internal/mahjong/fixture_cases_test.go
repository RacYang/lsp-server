package mahjong_test

import (
	"encoding/json"
	"os"
	"testing"

	"racoo.cn/lsp/internal/mahjong/hand"
	"racoo.cn/lsp/internal/mahjong/rules"
	_ "racoo.cn/lsp/internal/mahjong/sichuanxzdd" // 触发 init 注册川麻规则
	"racoo.cn/lsp/internal/mahjong/tile"
)

type huFixture struct {
	Case            string   `json:"case"`
	Hand            []string `json:"hand"`
	Win             string   `json:"win"`
	WantWin         bool     `json:"want_win"`
	WantMinTotalFan int      `json:"want_min_total_fan"`
}

// TestJSONSimpleHu 读取 testdata 夹具，验证规则接口与和牌判定链路。
func TestJSONSimpleHu(t *testing.T) {
	const rel = "testdata/simple_hu.json"
	b, err := os.ReadFile(rel) //nolint:gosec // G304：固定常量路径，仅读取仓库内夹具
	if err != nil {
		t.Fatalf("read json: %v", err)
	}
	var fx huFixture
	if err := json.Unmarshal(b, &fx); err != nil {
		t.Fatalf("json: %v", err)
	}
	h := hand.New()
	for _, s := range fx.Hand {
		ti, err := tile.Parse(s)
		if err != nil {
			t.Fatalf("parse %q: %v", s, err)
		}
		h.Add(ti)
	}
	winTile, err := tile.Parse(fx.Win)
	if err != nil {
		t.Fatalf("parse win: %v", err)
	}
	r := rules.MustGet("sichuan_xzdd")
	res, ok := r.CheckHu(h, winTile, rules.HuContext{})
	if ok != fx.WantWin {
		t.Fatalf("want_win=%v got=%v", fx.WantWin, ok)
	}
	if !ok {
		return
	}
	sc := r.ScoreFans(res, rules.ScoreContext{})
	if sc.Total < fx.WantMinTotalFan {
		t.Fatalf("fan total=%d want>=%d", sc.Total, fx.WantMinTotalFan)
	}
}
