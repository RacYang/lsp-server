package room

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMultiRuleDocumentationDoesNotDescribeLegacyFieldsAsRoomModel(t *testing.T) {
	t.Parallel()

	cases := []struct {
		path      string
		forbid    string
		mustHave  string
		rationale string
	}{
		{
			path:      "ROOM-FSM.md",
			forbid:    "广播换三张、定缺、开局。",
			mustHave:  "room 只维护通用等待态、行动窗口、`win_events`、`score_events`、opaque `rule_state`",
			rationale: "opening steps must be described as rule strategy output, not as universal room flow",
		},
		{
			path:      "ROOM-FSM.md",
			forbid:    "玩家胡牌后记录 `hued_seats` 并退出后续轮转",
			mustHave:  "`hued_seats` 不是 room 主模型",
			rationale: "win aftermath must be described through WinEvent/ScoreEvent and termination policy",
		},
		{
			path:      "adr/0039-sichuan-xuezhandaodi-authoritative-round-contract.md",
			forbid:    "换三张方向是 `RoundState.exchangeDirection` 的局共识字段",
			mustHave:  "四川规则包保存在 opaque `rule_state` 中",
			rationale: "Sichuan opening state must be documented as rule-private state projection",
		},
		{
			path:      "adr/0017-room-engine-and-settlement-boundary.md",
			forbid:    "承载 actor 串行状态、换三张、定缺、摸打、抢答、恢复 JSON 与结算推送",
			mustHave:  "统一记录 `WinEvent` / `ScoreEvent` 并调用规则策略生成 `SettlementResult`",
			rationale: "room engine documentation must not center Sichuan-specific flow",
		},
		{
			path:      "adr/0020-rules-deepening.md",
			forbid:    "`internal/service/room.RoundState` 使用 `[]xuezhandaodi.ScoreEntry` 保存胡分、杠分、退税和包牌事实",
			mustHave:  "当前实现改为通用 `ScoreEvent`",
			rationale: "historical Sichuan score entry must not be documented as current room state",
		},
		{
			path:      "adr/0040-composable-mahjong-rule-capabilities.md",
			forbid:    "旧内嵌字段只在反序列化时转成迁移输入",
			mustHave:  "旧内嵌字段会被反序列化拒绝",
			rationale: "hard-cut RuleState must reject legacy embedded fields",
		},
		{
			path:      "adr/0014-reconnect-session-and-snapshot-cutover.md",
			forbid:    "剩余牌墙、定缺、已累计番数",
			mustHave:  "`rule_id`、opaque `rule_state`、`win_events` 与 `score_events`",
			rationale: "round_json restore docs must describe generic facts and opaque rule state",
		},
		{
			path:      "PROTOCOL.md",
			forbid:    "`waiting_action` 表示服务端正在等待的动作类型，值域为 `exchange_three`、`que_men`、`discard`、`claim_window`、`tsumo_window`、`none` 或空值",
			mustHave:  "所有开局步骤统一为 `WAITING_REASON_OPENING`",
			rationale: "protocol action names are compatibility projection, not room phases",
		},
		{
			path:      "spec/player-journey.md",
			forbid:    "已胡座位（`hued_seats` 含）不参与摸打轮转",
			mustHave:  "本 spec 以默认规则 `sichuan_xuezhandaodi_huansanzhang` 的玩家旅程为主",
			rationale: "player journey must mark Sichuan-specific behavior as default-rule projection",
		},
	}

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()

			src := readDocForGuard(t, tc.path)
			require.NotContains(t, src, tc.forbid, tc.rationale)
			require.Contains(t, src, tc.mustHave, tc.rationale)
		})
	}
}

func readDocForGuard(t *testing.T, rel string) string {
	t.Helper()
	path := filepath.Join("..", "..", "..", "docs", rel)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}
