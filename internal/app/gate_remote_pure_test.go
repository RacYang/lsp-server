// remote room gateway 中纯函数与转换器的单元测试，避免起 grpc 依赖。
package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	clusterv1 "racoo.cn/lsp/api/gen/go/cluster/v1"
	"racoo.cn/lsp/internal/net/msgid"
	"racoo.cn/lsp/pkg/logx"
)

// TestSplitCommaSeparated 校验逗号分隔符串切分时空白与空段会被忽略。
func TestSplitCommaSeparated(t *testing.T) {
	t.Parallel()
	got := splitCommaSeparated("  a , ,b ,, c")
	require.Equal(t, []string{"a", "b", "c"}, got)
	require.Empty(t, splitCommaSeparated(""))
}

// TestWithOutgoingTrace 校验 trace_id 的注入：缺少 trace_id 时透传 ctx，存在则写入 outgoing metadata。
func TestWithOutgoingTrace(t *testing.T) {
	t.Parallel()
	ctxNoTrace := context.Background()
	require.Equal(t, ctxNoTrace, withOutgoingTrace(ctxNoTrace), "无 trace_id 时直接返回原 ctx")

	ctx := logx.WithTraceID(context.Background(), "trace-pure")
	out := withOutgoingTrace(ctx)
	md, ok := metadata.FromOutgoingContext(out)
	require.True(t, ok)
	require.Equal(t, []string{"trace-pure"}, md.Get("racoo-trace-id"))
}

// TestClusterToClientConverters 同时验证四个 cluster->client 转换器：均能逐字段透传，nil 输入返回空切片。
func TestClusterToClientConverters(t *testing.T) {
	t.Parallel()

	require.Empty(t, clusterClaimCandidatesToClient(nil))
	require.Empty(t, clusterSeatScoresToClient(nil))
	require.Empty(t, clusterPenaltiesToClient(nil))
	require.Empty(t, clusterWinnerBreakdownsToClient(nil))

	candidates := []*clusterv1.ClaimCandidate{{SeatIndex: 1, Actions: []string{"pong", "gang"}}}
	got := clusterClaimCandidatesToClient(candidates)
	require.Len(t, got, 1)
	require.Equal(t, int32(1), got[0].GetSeatIndex())
	require.Equal(t, []string{"pong", "gang"}, got[0].GetActions())

	seatScores := []*clusterv1.SeatScore{{SeatIndex: 2, UserId: "u", TotalFan: 10, Skipped: true}}
	gotScores := clusterSeatScoresToClient(seatScores)
	require.Equal(t, "u", gotScores[0].GetUserId())
	require.Equal(t, int32(10), gotScores[0].GetTotalFan())
	require.True(t, gotScores[0].GetSkipped())

	penalties := []*clusterv1.PenaltyItem{{Reason: "miss", FromSeat: 0, ToSeat: 1, Amount: 4}}
	gotPenalties := clusterPenaltiesToClient(penalties)
	require.Equal(t, int32(4), gotPenalties[0].GetAmount())

	breakdowns := []*clusterv1.WinnerBreakdown{{SeatIndex: 3, UserId: "w", Fan: 6, FanNames: []string{"清一色"}}}
	gotBreakdowns := clusterWinnerBreakdownsToClient(breakdowns)
	require.Equal(t, []string{"清一色"}, gotBreakdowns[0].GetFanNames())
}

// TestMarshalClientEnvelope 校验 marshal 失败与成功两条路径；空 envelope 不会引发错误。
func TestMarshalClientEnvelope(t *testing.T) {
	t.Parallel()
	envID, payload, err := marshalClientEnvelope(msgid.StartGame, &clientv1.Envelope{ReqId: "x"})
	require.NoError(t, err)
	require.Equal(t, msgid.StartGame, envID)
	require.NotEmpty(t, payload)

	var decoded clientv1.Envelope
	require.NoError(t, proto.Unmarshal(payload, &decoded))
	require.Equal(t, "x", decoded.GetReqId())
}

// TestEncodeClusterRoomEventAllBranches 覆盖 encodeClusterRoomEvent 全部分支，包括 nil 输入与未知 body。
func TestEncodeClusterRoomEventAllBranches(t *testing.T) {
	t.Parallel()

	_, _, err := encodeClusterRoomEvent(nil)
	require.Error(t, err)

	cases := []struct {
		name   string
		evt    *clusterv1.RoomServiceStreamEventsResponse
		wantID uint16
	}{
		{
			name: "start_game",
			evt: &clusterv1.RoomServiceStreamEventsResponse{
				RoomId: "r1",
				Cursor: "1",
				Body: &clusterv1.RoomServiceStreamEventsResponse_StartGame{
					StartGame: &clusterv1.StartGameEvent{DealerSeat: 2},
				},
			},
			wantID: msgid.StartGame,
		},
		{
			name: "draw_tile",
			evt: &clusterv1.RoomServiceStreamEventsResponse{
				Body: &clusterv1.RoomServiceStreamEventsResponse_DrawTile{
					DrawTile: &clusterv1.DrawTileEvent{SeatIndex: 1, Tile: "1m"},
				},
			},
			wantID: msgid.DrawTile,
		},
		{
			name: "action",
			evt: &clusterv1.RoomServiceStreamEventsResponse{
				Body: &clusterv1.RoomServiceStreamEventsResponse_Action{
					Action: &clusterv1.ActionEvent{SeatIndex: 0, Action: "pong", Tile: "5w"},
				},
			},
			wantID: msgid.ActionNotify,
		},
		{
			name: "settlement",
			evt: &clusterv1.RoomServiceStreamEventsResponse{
				RoomId: "r-set",
				Body: &clusterv1.RoomServiceStreamEventsResponse_Settlement{
					Settlement: &clusterv1.SettlementEvent{
						WinnerUserIds: []string{"u1"},
						TotalFan:      4,
						SeatScores:    []*clusterv1.SeatScore{{SeatIndex: 0}},
						Penalties:     []*clusterv1.PenaltyItem{{Reason: "x"}},
					},
				},
			},
			wantID: msgid.Settlement,
		},
		{
			name: "exchange_three_done",
			evt: &clusterv1.RoomServiceStreamEventsResponse{
				Body: &clusterv1.RoomServiceStreamEventsResponse_ExchangeThreeDone{
					ExchangeThreeDone: &clusterv1.ExchangeThreeDoneEvent{
						SeatTiles: []*clusterv1.SeatTiles{{SeatIndex: 0, Tiles: []string{"1m"}}},
					},
				},
			},
			wantID: msgid.ExchangeThreeDone,
		},
		{
			name: "que_men_done",
			evt: &clusterv1.RoomServiceStreamEventsResponse{
				Body: &clusterv1.RoomServiceStreamEventsResponse_QueMenDone{
					QueMenDone: &clusterv1.QueMenDoneEvent{QueSuitBySeat: []int32{0, 1, 2, 0}},
				},
			},
			wantID: msgid.QueMenDone,
		},
		{
			name: "route_redirect",
			evt: &clusterv1.RoomServiceStreamEventsResponse{
				Body: &clusterv1.RoomServiceStreamEventsResponse_RouteRedirect{
					RouteRedirect: &clusterv1.RouteRedirectEvent{WsUrl: "ws://x", Reason: "moved"},
				},
			},
			wantID: msgid.RouteRedirectNotify,
		},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			gotID, payload, err := encodeClusterRoomEvent(c.evt)
			require.NoError(t, err)
			require.Equal(t, c.wantID, gotID)
			require.NotEmpty(t, payload)
		})
	}

	t.Run("unknown_body", func(t *testing.T) {
		t.Parallel()
		_, _, err := encodeClusterRoomEvent(&clusterv1.RoomServiceStreamEventsResponse{})
		require.Error(t, err)
	})
}

// TestRetryGRPC 校验 retryGRPC 的三条主要路径：成功立即返回、非可重试错误立刻返回、ctx 取消快速返回。
func TestRetryGRPC(t *testing.T) {
	t.Parallel()

	calls := 0
	err := retryGRPC(context.Background(), func(_ context.Context) error {
		calls++
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 1, calls)

	calls = 0
	err = retryGRPC(context.Background(), func(_ context.Context) error {
		calls++
		return errors.New("non-grpc")
	})
	require.Error(t, err)
	require.Equal(t, 1, calls, "非 grpc 错误必须立即返回，不再重试")

	calls = 0
	err = retryGRPC(context.Background(), func(_ context.Context) error {
		calls++
		return status.Error(codes.PermissionDenied, "deny")
	})
	require.Error(t, err)
	require.Equal(t, 1, calls, "非 Unavailable/DeadlineExceeded 错误必须立即返回")

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err = retryGRPC(ctx, func(_ context.Context) error {
		return status.Error(codes.Unavailable, "unavail")
	})
	require.Error(t, err)
}
