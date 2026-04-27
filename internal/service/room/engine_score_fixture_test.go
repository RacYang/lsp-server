package room

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"racoo.cn/lsp/internal/mahjong/fan"
	"racoo.cn/lsp/internal/mahjong/rules"
	"racoo.cn/lsp/internal/mahjong/sichuanxzdd"
	"racoo.cn/lsp/internal/mahjong/tile"
)

type scoreLedgerFixtureFile struct {
	Cases []scoreLedgerFixtureCase `yaml:"cases"`
}

type scoreLedgerFixtureCase struct {
	Name           string               `yaml:"name"`
	Winner         int                  `yaml:"winner"`
	Source         string               `yaml:"source"`
	Payer          int                  `yaml:"payer"`
	Fan            int                  `yaml:"fan"`
	ExpectEntries  []fixtureLedgerEntry `yaml:"expect_entries"`
	ExpectFanName  string               `yaml:"expect_fan_name"`
	Seat           int                  `yaml:"seat"`
	GangKind       string               `yaml:"gang_kind"`
	AmountPerPayer int32                `yaml:"amount_per_payer"`
}

type fixtureLedgerEntry struct {
	From   int   `yaml:"from"`
	To     int   `yaml:"to"`
	Amount int32 `yaml:"amount"`
}

func TestScoreLedgerYAMLFixtures(t *testing.T) {
	var file scoreLedgerFixtureFile
	data, err := os.ReadFile(filepath.Join("..", "..", "mahjong", "sichuanxzdd", "testdata", "score_ledger_cases.yaml"))
	if err != nil {
		t.Fatalf("read score ledger fixture: %v", err)
	}
	if err := yaml.Unmarshal(data, &file); err != nil {
		t.Fatalf("unmarshal score ledger fixture: %v", err)
	}
	for _, tc := range file.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			rs := scoreRoundState()
			if tc.GangKind != "" {
				appendGangEntries(rs, tc.Seat, tile.Must(tile.SuitCharacters, 5), fixtureLedgerGangKind(t, tc.GangKind), -1)
				for _, entry := range rs.ledger {
					if entry.Amount != tc.AmountPerPayer {
						t.Fatalf("amount = %d, want %d", entry.Amount, tc.AmountPerPayer)
					}
				}
				return
			}
			breakdown := fan.Breakdown{}
			breakdown.Add(fan.KindPingHu, tc.Fan, "平胡")
			appendHuEntries(rs, tc.Winner, tc.Fan, fixtureHuSource(t, tc.Source), tc.Payer, breakdown)
			for _, want := range tc.ExpectEntries {
				if !ledgerHasEntry(rs.ledger, want) {
					t.Fatalf("missing ledger entry %+v in %+v", want, rs.ledger)
				}
			}
			if tc.ExpectFanName != "" && (len(rs.ledger) == 0 || !containsString(rs.ledger[0].FanNames, tc.ExpectFanName)) {
				t.Fatalf("missing fan name %s in %+v", tc.ExpectFanName, rs.ledger)
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

func fixtureLedgerGangKind(t *testing.T, raw string) rules.GangKind {
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

func ledgerHasEntry(entries []sichuanxzdd.ScoreEntry, want fixtureLedgerEntry) bool {
	for _, entry := range entries {
		if entry.FromSeat == want.From && entry.ToSeat == want.To && entry.Amount == want.Amount {
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
