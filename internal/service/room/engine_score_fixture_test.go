package room

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"racoo.cn/lsp/internal/mahjong/fan"
	"racoo.cn/lsp/internal/mahjong/rules"
	"racoo.cn/lsp/internal/mahjong/tile"
)

type scoreEventFixtureFile struct {
	Cases []scoreEventFixtureCase `yaml:"cases"`
}

type scoreEventFixtureCase struct {
	Name           string              `yaml:"name"`
	Winner         int                 `yaml:"winner"`
	Source         string              `yaml:"source"`
	Payer          int                 `yaml:"payer"`
	Fan            int                 `yaml:"fan"`
	ExpectEntries  []fixtureScoreEvent `yaml:"expect_events"`
	ExpectFanName  string              `yaml:"expect_fan_name"`
	Seat           int                 `yaml:"seat"`
	GangKind       string              `yaml:"gang_kind"`
	AmountPerPayer int32               `yaml:"amount_per_payer"`
}

type fixtureScoreEvent struct {
	From   int   `yaml:"from"`
	To     int   `yaml:"to"`
	Amount int32 `yaml:"amount"`
}

func TestScoreEventYAMLFixtures(t *testing.T) {
	var file scoreEventFixtureFile
	data, err := os.ReadFile(filepath.Join("..", "..", "mahjong", "sichuan", "xuezhandaodi", "testdata", "score_event_cases.yaml"))
	if err != nil {
		t.Fatalf("read score event fixture: %v", err)
	}
	if err := yaml.Unmarshal(data, &file); err != nil {
		t.Fatalf("unmarshal score event fixture: %v", err)
	}
	for _, tc := range file.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			rs := scoreRoundState()
			if tc.GangKind != "" {
				appendGangEntries(rs, SeatFromInt(tc.Seat), tile.Must(tile.SuitCharacters, 5), fixtureScoreEventGangKind(t, tc.GangKind), SeatInvalid)
				for _, entry := range rs.scoreEvents {
					if entry.Amount != tc.AmountPerPayer {
						t.Fatalf("amount = %d, want %d", entry.Amount, tc.AmountPerPayer)
					}
				}
				return
			}
			breakdown := fan.Breakdown{}
			breakdown.Add(fan.KindPingHu, tc.Fan, "平胡")
			appendHuEntries(rs, SeatFromInt(tc.Winner), tc.Fan, fixtureHuSource(t, tc.Source), SeatFromInt(tc.Payer), breakdown)
			for _, want := range tc.ExpectEntries {
				if !scoreEventsHaveEntry(rs.scoreEvents, want) {
					t.Fatalf("missing score event %+v in %+v", want, rs.scoreEvents)
				}
			}
			if tc.ExpectFanName != "" && (len(rs.scoreEvents) == 0 || !containsString(rs.scoreEvents[0].FanNames, tc.ExpectFanName)) {
				t.Fatalf("missing fan name %s in %+v", tc.ExpectFanName, rs.scoreEvents)
			}
		})
	}
}

func fixtureHuSource(t *testing.T, raw string) rules.HuSource {
	t.Helper()
	switch raw {
	case "tsumo":
		return rules.HuSourceTsumo
	case "discard":
		return rules.HuSourceDiscard
	case "qiang_gang":
		return rules.HuSourceQiangGang
	default:
		t.Fatalf("unknown hu source %s", raw)
		return rules.HuSourceUnspecified
	}
}

func fixtureScoreEventGangKind(t *testing.T, raw string) rules.GangKind {
	t.Helper()
	switch raw {
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

func scoreEventsHaveEntry(entries []rules.ScoreEvent, want fixtureScoreEvent) bool {
	for _, entry := range entries {
		if entry.FromSeat == SeatFromInt(want.From) && entry.ToSeat == SeatFromInt(want.To) && entry.Amount == want.Amount {
			return true
		}
	}
	return false
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
