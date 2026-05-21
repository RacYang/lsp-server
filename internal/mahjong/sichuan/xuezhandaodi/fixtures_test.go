package xuezhandaodi

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	domainroom "racoo.cn/lsp/internal/domain/room"
	"racoo.cn/lsp/internal/mahjong/hand"
	"racoo.cn/lsp/internal/mahjong/hu"
	"racoo.cn/lsp/internal/mahjong/rules"
	"racoo.cn/lsp/internal/mahjong/tile"
)

type fanFixtureFile struct {
	Cases []fanFixtureCase `yaml:"cases"`
}

type fanFixtureCase struct {
	Name         string              `yaml:"name"`
	Win          []string            `yaml:"win"`
	ScoreContext fixtureScoreContext `yaml:"score_context"`
	GangRecords  []fixtureGangRecord `yaml:"gang_records"`
	ExpectFans   []string            `yaml:"expect_fans"`
}

type fixtureScoreContext struct {
	HuSeat               int  `yaml:"hu_seat"`
	DealerSeat           int  `yaml:"dealer_seat"`
	IsTsumo              bool `yaml:"is_tsumo"`
	IsOpeningDraw        bool `yaml:"is_opening_draw"`
	IsDealerFirstDiscard bool `yaml:"is_dealer_first_discard"`
	IsGangShangPao       bool `yaml:"is_gang_shang_pao"`
}

type fixtureGangRecord struct {
	Seat int    `yaml:"seat"`
	Kind string `yaml:"kind"`
	Tile string `yaml:"tile"`
}

func TestFanDeepeningYAMLFixtures(t *testing.T) {
	var file fanFixtureFile
	readYAMLFixture(t, "fan_deepening_cases.yaml", &file)
	x := newRule(IDHuansanzhang, true)
	for _, tc := range file.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			ctx := rules.ScoreContext{
				HuSeat:               domainroom.SeatFromInt(tc.ScoreContext.HuSeat),
				DealerSeat:           domainroom.SeatFromInt(tc.ScoreContext.DealerSeat),
				IsTsumo:              tc.ScoreContext.IsTsumo,
				IsOpeningDraw:        tc.ScoreContext.IsOpeningDraw,
				IsDealerFirstDiscard: tc.ScoreContext.IsDealerFirstDiscard,
				IsGangShangPao:       tc.ScoreContext.IsGangShangPao,
				GangRecords:          fixtureGangRecords(t, tc.GangRecords),
			}
			result := rules.HuResult{Win: countsFromFixtureTiles(t, tc.Win)}
			breakdown := testScoreWin(x, result, ctx)
			labels := map[string]bool{}
			for _, item := range breakdown.Items {
				labels[item.Label] = true
			}
			for _, want := range tc.ExpectFans {
				if !labels[want] {
					t.Fatalf("missing fan %s in %+v", want, breakdown.Items)
				}
			}
		})
	}
}

type chaDaJiaoFixtureFile struct {
	Cases []chaDaJiaoFixtureCase `yaml:"cases"`
}

type chaDaJiaoFixtureCase struct {
	Name          string   `yaml:"name"`
	Hand          []string `yaml:"hand"`
	QueSuit       string   `yaml:"que_suit"`
	Winners       []int    `yaml:"winners"`
	ExpectPenalty bool     `yaml:"expect_penalty"`
	ExpectReason  string   `yaml:"expect_reason"`
}

func TestChaDaJiaoYAMLFixtures(t *testing.T) {
	var file chaDaJiaoFixtureFile
	readYAMLFixture(t, "cha_da_jiao_cases.yaml", &file)
	playerIDs := [4]string{"u0", "u1", "u2", "u3"}
	for _, tc := range file.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			hands := []*hand.Hand{handFromFixtureTiles(t, tc.Hand), hand.New(), hand.New(), hand.New()}
			queBySeat := []int32{int32(fixtureSuit(t, tc.QueSuit)), int32(tile.SuitCharacters), int32(tile.SuitDots), int32(tile.SuitBamboo)}
			_, penalties, _, _ := BuildSettlement(playerIDs, hands, queBySeat, nil, fixtureSeats(tc.Winners))
			var sawSeatPenalty bool
			var sawReason bool
			for _, penalty := range penalties {
				if penalty.GetFromSeat() == 0 && penalty.GetReason() == ReasonChaDaJiao {
					sawSeatPenalty = true
				}
				if tc.ExpectReason != "" && penalty.GetFromSeat() == 0 && penalty.GetReason() == tc.ExpectReason {
					sawReason = true
				}
			}
			if sawSeatPenalty != tc.ExpectPenalty {
				t.Fatalf("cha da jiao penalty = %v, want %v, penalties=%+v", sawSeatPenalty, tc.ExpectPenalty, penalties)
			}
			if tc.ExpectReason != "" && !sawReason {
				t.Fatalf("missing reason %s in %+v", tc.ExpectReason, penalties)
			}
		})
	}
}

type gangRefundFixtureFile struct {
	Cases []gangRefundFixtureCase `yaml:"cases"`
}

type gangRefundFixtureCase struct {
	Name          string              `yaml:"name"`
	SkippedSeat   int                 `yaml:"skipped_seat"`
	NoTingSeat    int                 `yaml:"no_ting_seat"`
	Draw          bool                `yaml:"draw"`
	ScoreEvents   []fixtureScoreEvent `yaml:"score_events"`
	ExpectPenalty fixturePenalty      `yaml:"expect_penalty"`
}

type fixtureScoreEvent struct {
	Reason string `yaml:"reason"`
	From   int    `yaml:"from"`
	To     int    `yaml:"to"`
	Amount int32  `yaml:"amount"`
}

type fixturePenalty struct {
	Reason string `yaml:"reason"`
	From   int32  `yaml:"from"`
	To     int32  `yaml:"to"`
	Amount int32  `yaml:"amount"`
}

func TestGangRefundYAMLFixtures(t *testing.T) {
	var file gangRefundFixtureFile
	readYAMLFixture(t, "gang_refund_cases.yaml", &file)
	playerIDs := [4]string{"u0", "u1", "u2", "u3"}
	for _, tc := range file.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			hands := []*hand.Hand{hand.New(), hand.New(), hand.New(), hand.New()}
			queBySeat := []int32{int32(tile.SuitCharacters), int32(tile.SuitDots), int32(tile.SuitBamboo), int32(tile.SuitCharacters)}
			winners := []int{0}
			if tc.Draw {
				winners = nil
			}
			if tc.SkippedSeat > 0 {
				queBySeat[tc.SkippedSeat] = int32(tile.SuitCharacters)
				hands[tc.SkippedSeat] = handFromFixtureTiles(t, []string{"m1"})
			}
			if tc.NoTingSeat > 0 {
				hands[tc.NoTingSeat] = handFromFixtureTiles(t, []string{"m1", "m3", "m5", "m7", "m9"})
			}
			scoreEvents := make([]rules.ScoreEvent, 0, len(tc.ScoreEvents))
			for _, entry := range tc.ScoreEvents {
				scoreEvents = append(scoreEvents, rules.ScoreEvent{Reason: entry.Reason, FromSeat: domainroom.SeatFromInt(entry.From), ToSeat: domainroom.SeatFromInt(entry.To), Amount: entry.Amount, WinnerSeat: domainroom.SeatInvalid})
			}
			_, penalties, _, _ := BuildSettlement(playerIDs, hands, queBySeat, scoreEvents, fixtureSeats(winners))
			for _, penalty := range penalties {
				if penalty.GetReason() == tc.ExpectPenalty.Reason &&
					penalty.GetFromSeat() == tc.ExpectPenalty.From &&
					penalty.GetToSeat() == tc.ExpectPenalty.To &&
					penalty.GetAmount() == tc.ExpectPenalty.Amount {
					return
				}
			}
			t.Fatalf("missing expected penalty %+v in %+v", tc.ExpectPenalty, penalties)
		})
	}
}

func readYAMLFixture(t *testing.T, name string, out any) {
	t.Helper()
	switch name {
	case "fan_deepening_cases.yaml", "cha_da_jiao_cases.yaml", "gang_refund_cases.yaml":
	default:
		t.Fatalf("unexpected fixture name %s", name)
	}
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	if err := yaml.Unmarshal(data, out); err != nil {
		t.Fatalf("unmarshal fixture %s: %v", name, err)
	}
}

func fixtureGangRecords(t *testing.T, records []fixtureGangRecord) []rules.GangRecord {
	t.Helper()
	out := make([]rules.GangRecord, 0, len(records))
	for _, record := range records {
		ti := tile.Tile(0)
		if record.Tile != "" {
			var err error
			ti, err = tile.Parse(record.Tile)
			if err != nil {
				t.Fatalf("parse gang tile %s: %v", record.Tile, err)
			}
		}
		out = append(out, rules.GangRecord{Seat: domainroom.SeatFromInt(record.Seat), Kind: fixtureGangKind(t, record.Kind), Tile: ti})
	}
	return out
}

func fixtureSeats(in []int) []domainroom.Seat {
	out := make([]domainroom.Seat, 0, len(in))
	for _, seat := range in {
		out = append(out, domainroom.SeatFromInt(seat))
	}
	return out
}

func fixtureGangKind(t *testing.T, raw string) rules.GangKind {
	t.Helper()
	switch raw {
	case "", "unspecified":
		return rules.GangKindUnspecified
	case "ming":
		return rules.GangKindMing
	case "an":
		return rules.GangKindAn
	case "bu":
		return rules.GangKindBu
	default:
		t.Fatalf("unknown gang kind %s", raw)
		return rules.GangKindUnspecified
	}
}

func countsFromFixtureTiles(t *testing.T, texts []string) hu.Counts {
	t.Helper()
	var c hu.Counts
	for _, text := range texts {
		ti, err := tile.Parse(text)
		if err != nil {
			t.Fatalf("parse tile %s: %v", text, err)
		}
		c[ti.Index()]++
	}
	return c
}

func handFromFixtureTiles(t *testing.T, texts []string) *hand.Hand {
	t.Helper()
	h := hand.New()
	for _, text := range texts {
		ti, err := tile.Parse(text)
		if err != nil {
			t.Fatalf("parse tile %s: %v", text, err)
		}
		h.Add(ti)
	}
	return h
}

func fixtureSuit(t *testing.T, raw string) tile.Suit {
	t.Helper()
	switch raw {
	case "m":
		return tile.SuitCharacters
	case "p":
		return tile.SuitDots
	case "s":
		return tile.SuitBamboo
	default:
		t.Fatalf("unknown suit %s", raw)
		return tile.SuitCharacters
	}
}
