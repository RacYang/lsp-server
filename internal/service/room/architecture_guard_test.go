package room

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRoomEngineArchitectureGuard(t *testing.T) {
	t.Parallel()

	for _, file := range roomGoFiles(t) {
		base := filepath.Base(file)
		data, err := os.ReadFile(file)
		require.NoError(t, err)
		src := string(data)

		require.NotContains(t, src, "internal/mahjong/sichuan/xuezhandaodi", "%s must not import concrete Sichuan rules", base)
		require.NotContains(t, src, "internal/mahjong/guobiao", "%s must not import concrete Guobiao rules", base)
		require.NotContains(t, src, "rule.BuildWall(", "%s must use TileSetPolicy, not Rule.BuildWall", base)
		require.NotContains(t, src, "rule.CheckHu(", "%s must use WinPolicy, not Rule.CheckHu", base)
		require.NotContains(t, src, "rs.rule.CheckHu(", "%s must use WinPolicy, not Rule.CheckHu", base)
		require.NotContains(t, src, "rule.ScoreFans(", "%s must use ScoringPolicy, not Rule.ScoreFans", base)
		require.NotContains(t, src, "rs.rule.ScoreFans(", "%s must use ScoringPolicy, not Rule.ScoreFans", base)
		require.NotContains(t, src, "rule.GameOver(", "%s must use TerminationPolicy, not Rule.GameOver", base)
		require.NotContains(t, src, "rs.rule.GameOver(", "%s must use TerminationPolicy, not Rule.GameOver", base)
		require.NotContains(t, src, "tileIsQueSuit", "%s must not contain Sichuan missing-suit checks", base)
		require.NotContains(t, src, "filterQueSuit", "%s must not contain Sichuan missing-suit claim filters", base)
		require.NotContains(t, src, "MissingSuitBySeat", "%s must not read rule-private missing suit state", base)
		require.NotContains(t, src, "ExchangeSubmitted", "%s must use generic opening submitted projection", base)
		require.NotContains(t, src, "QueSubmitted", "%s must use generic opening submitted projection", base)
		require.NotContains(t, src, "OpeningSelections", "%s must not read rule-private opening selection state", base)
		require.NotContains(t, src, "OpeningDirection", "%s must not read rule-private opening direction state", base)
		require.NotContains(t, src, "LegacyRuleState", "%s must not depend on legacy rule state", base)
		require.NotContains(t, src, "MigrateLegacyRuleState", "%s must not migrate legacy rule state", base)
		require.NotContains(t, src, "waitingExchange", "%s must use generic opening wait state", base)
		require.NotContains(t, src, "waitingQueMen", "%s must use generic opening wait state", base)
		require.NotContains(t, src, "WaitingExchange", "%s must persist generic opening wait state", base)
		require.NotContains(t, src, "WaitingQueMen", "%s must persist generic opening wait state", base)
		require.NotContains(t, src, "ReasonExchangeThree", "%s must use generic opening waiting reason", base)
		require.NotContains(t, src, "ReasonQueMen", "%s must use generic opening waiting reason", base)
		require.NotContains(t, src, "WAITING_REASON_EXCHANGE_THREE", "%s must use generic opening waiting reason", base)
		require.NotContains(t, src, "WAITING_REASON_QUE_MEN", "%s must use generic opening waiting reason", base)
		require.NotContains(t, src, `"exchange_three"`, "%s must not hard-code Sichuan opening action names", base)
		require.NotContains(t, src, `"que_men"`, "%s must not hard-code Sichuan opening action names", base)
		require.NotContains(t, src, "PHASE_EXCHANGE", "%s must use generic opening phase", base)
		require.NotContains(t, src, "PHASE_QUE_MEN", "%s must use generic opening phase", base)
		require.NotContains(t, src, "autoPhaseForOpeningStep", "%s must not map opening action names to phases", base)
		require.NotContains(t, src, "surrenderExchangeSeat", "%s must surrender opening seats through generic OpeningPolicy", base)
		require.NotContains(t, src, "surrenderQueMenSeat", "%s must surrender opening seats through generic OpeningPolicy", base)
		require.NotContains(t, src, "applyExchangeOpeningByTimeout", "%s must timeout opening actions through generic OpeningPolicy", base)
		require.NotContains(t, src, "cmdExchangeThree", "%s must use generic opening action actor command", base)
		require.NotContains(t, src, "cmdQueMen", "%s must use generic opening action actor command", base)
		require.NotContains(t, src, "ApplyExchangeThree", "%s must route opening submissions through generic opening action", base)
		require.NotContains(t, src, "ApplyQueMen", "%s must route opening submissions through generic opening action", base)
		require.NotContains(t, src, "KindExchangeThreeDone", "%s must use generic opening done notification kind", base)
		require.NotContains(t, src, "KindQueMenDone", "%s must use generic opening done notification kind", base)
		require.NotContains(t, src, "ExchangeThreeDoneNotify", "%s must use generic OpeningDoneNotify", base)
		require.NotContains(t, src, "QueMenDoneNotify", "%s must use generic OpeningDoneNotify", base)
		require.NotContains(t, src, "HuAftermathPolicy", "%s must get hu aftermath from CapabilitySet.Turn", base)
		require.NotContains(t, src, "SettlementBuilder", "%s must get settlement from CapabilitySet.Settlement", base)
		require.NotContains(t, src, "rs.rule.(rules.", "%s must not type-assert rule runtime strategies", base)
		if base != "engine.go" {
			require.NotContains(t, src, "rules.CapabilitiesOf(rs.rule)", "%s must use RoundState.caps instead of re-querying rule capabilities", base)
		}
		require.NotContains(t, src, "rules.StandardSelfActionPolicy{}", "%s must get self-action legality from CapabilitySet.SelfActions, not room fallback", base)
		require.NotContains(t, src, "caps.Settlement != nil", "%s must require SettlementPolicy at capability assembly time", base)
		require.NotContains(t, src, "rs.caps.Settlement != nil", "%s must require SettlementPolicy at capability assembly time", base)
		require.NotContains(t, src, "caps.Turn == nil", "%s must require TurnFlow at capability assembly time", base)
		require.NotContains(t, src, "rs.caps.Turn == nil", "%s must require TurnFlow at capability assembly time", base)
		require.NotContains(t, src, "caps.State != nil", "%s must require RuleStateCodec at capability assembly time", base)
		require.NotContains(t, src, "rs.caps.State != nil", "%s must require RuleStateCodec at capability assembly time", base)
		require.NotContains(t, src, "caps.StateView == nil", "%s must require RuleStateProjector at capability assembly time", base)
		require.NotContains(t, src, "rs.caps.StateView == nil", "%s must require RuleStateProjector at capability assembly time", base)
		require.NotContains(t, src, "legacyClaimPriority", "%s must use rule claim priority naming, not legacy priority naming", base)
		require.NotContains(t, src, "rules.ScoreEntry", "%s must use generic ScoreEvent, not legacy score entry", base)
		require.NotContains(t, src, "ScoreEntry", "%s must use generic ScoreEvent, not legacy score entry", base)
		require.NotContains(t, src, "migratePersistToCurrent", "%s must reject non-current persist schema instead of migrating it", base)
		require.NotContains(t, src, "QueSuitsFromPersistJSON", "%s must project que suits from restored RoundState, not raw JSON", base)
		if base == "engine_auto.go" {
			require.NotContains(t, src, "exchangeThree(", "%s must not hard-code Sichuan exchange flow", base)
			require.NotContains(t, src, "chooseExchangeTiles(", "%s must let OpeningPolicy choose auto exchange tiles", base)
			require.NotContains(t, src, "chooseQueSuit(", "%s must let OpeningPolicy choose missing suit", base)
		}
		for _, tag := range []string{
			`json:"que_by_seat`,
			`json:"hued_seats`,
			`json:"winner_seat`,
			`json:"winner_seats`,
			`json:"ledger`,
			`json:"total_fan_by_seat`,
			`json:"exchange_tiles`,
			`json:"exchange_done`,
			`json:"que_done`,
			`json:"exchange_dir`,
			`json:"waiting_exchange`,
			`json:"waiting_que_men`,
		} {
			require.NotContains(t, src, tag, "%s must not write or restore legacy persist JSON tag %s", base, tag)
		}

		if isCompatRoomFile(base) {
			continue
		}
		require.NotContains(t, src, "QueBySeat", "%s must keep legacy que fields in compat/persist only", base)
		require.NotContains(t, src, "Ledger", "%s must use ScoreEvent naming for scoring facts", base)
	}
}

func TestCommonRulesDoNotDeclareSichuanOpeningActions(t *testing.T) {
	t.Parallel()

	files, err := filepath.Glob(filepath.Join("..", "..", "mahjong", "rules", "*.go"))
	require.NoError(t, err)
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		base := filepath.Base(file)
		data, err := os.ReadFile(file)
		require.NoError(t, err)
		src := string(data)
		require.NotContains(t, src, `"exchange_three"`, "%s must not declare Sichuan opening action names", base)
		require.NotContains(t, src, `"que_men"`, "%s must not declare Sichuan opening action names", base)
		require.NotContains(t, src, "OpeningActionExchangeThree", "%s must keep opening action names rule-owned", base)
		require.NotContains(t, src, "OpeningActionQueMen", "%s must keep opening action names rule-owned", base)
	}
}

func TestOpeningClientRuntimeDoesNotModelSichuanPhases(t *testing.T) {
	t.Parallel()

	for _, file := range repoGoFiles(t, "cmd/cli") {
		base := filepath.Base(file)
		src := readFileForGuard(t, file)
		require.NotContains(t, src, "PhaseExchange", "%s must use generic PhaseOpening", base)
		require.NotContains(t, src, "PhaseQueMen", "%s must use generic PhaseOpening", base)
	}
	for _, rel := range []string{
		"internal/service/bot/supervisor.go",
		"internal/bot/runner.go",
	} {
		src := readRepoFile(t, rel)
		require.NotContains(t, src, `"exchange_three"`, "%s must not branch runtime scheduling on Sichuan opening action names", rel)
		require.NotContains(t, src, `"que_men"`, "%s must not branch runtime scheduling on Sichuan opening action names", rel)
	}
}

func TestOpeningLegacyProtocolDefinitionsRemoved(t *testing.T) {
	t.Parallel()

	cases := []struct {
		path   string
		forbid []string
	}{
		{
			path: "api/proto/client/v1/messages.proto",
			forbid: []string{
				"PHASE_EXCHANGE =",
				"PHASE_QUE_MEN =",
				"ExchangeThreeRequest exchange_three_req =",
				"ExchangeThreeResponse exchange_three_resp =",
				"ExchangeThreeDoneNotify exchange_three_done =",
				"QueMenRequest que_men_req =",
				"QueMenResponse que_men_resp =",
				"QueMenDoneNotify que_men_done =",
				"message ExchangeThreeRequest",
				"message ExchangeThreeResponse",
				"message ExchangeThreeDoneNotify",
				"message QueMenRequest",
				"message QueMenResponse",
				"message QueMenDoneNotify",
			},
		},
		{
			// 已由集群 proto 合并为统一服务 proto（地基一重构）
			path: "api/proto/v1/service.proto",
			forbid: []string{
				"PHASE_EXCHANGE =",
				"PHASE_QUE_MEN =",
				"ExchangeThreeEvent exchange_three =",
				"QueMenEvent que_men =",
				"ExchangeThreeDoneEvent exchange_three_done =",
				"QueMenDoneEvent que_men_done =",
				"message ExchangeThreeEvent",
				"message QueMenEvent",
				"message ExchangeThreeDoneEvent",
				"message QueMenDoneEvent",
			},
		},
		{
			path: "internal/protocol/msgid.go",
			forbid: []string{
				"ExchangeThreeReq uint16",
				"ExchangeThreeResp uint16",
				"ExchangeThreeDone uint16",
				"QueMenReq uint16",
				"QueMenResp uint16",
				"QueMenDone uint16",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()
			src := readRepoFile(t, tc.path)
			for _, forbidden := range tc.forbid {
				require.NotContains(t, src, forbidden, "%s must expose only generic opening_action/opening_done", tc.path)
			}
		})
	}
}

func roomGoFiles(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(".", "*.go"))
	require.NoError(t, err)
	out := make([]string, 0, len(files))
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		out = append(out, file)
	}
	return out
}

func readRepoFile(t *testing.T, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", rel))
	require.NoError(t, err)
	return string(data)
}

func readFileForGuard(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

func repoGoFiles(t *testing.T, rel string) []string {
	t.Helper()
	root := filepath.Join("..", "..", "..", rel)
	var out []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		require.NoError(t, err)
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		out = append(out, path)
		return nil
	})
	require.NoError(t, err)
	return out
}

func isCompatRoomFile(base string) bool {
	switch base {
	case "engine.go", "engine_persist.go", "engine_contract.go":
		return true
	default:
		return false
	}
}
