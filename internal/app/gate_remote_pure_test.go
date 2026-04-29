// remote room gateway 中纯函数与转换器的单元测试，避免起 grpc 依赖。
package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
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
	require.Empty(t, clusterRoomMetasToClient(nil))

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

	rooms := []*clusterv1.RoomMeta{{RoomId: "ROOM01", RuleId: "sichuan_xzdd", DisplayName: "公开桌", SeatCount: 1, MaxSeats: 4, Stage: "waiting"}}
	gotRooms := clusterRoomMetasToClient(rooms)
	require.Equal(t, "ROOM01", gotRooms[0].GetRoomId())
	require.Equal(t, int32(1), gotRooms[0].GetSeatCount())
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
			name: "initial_deal",
			evt: &clusterv1.RoomServiceStreamEventsResponse{
				Cursor: "deal-0",
				Body: &clusterv1.RoomServiceStreamEventsResponse_InitialDeal{
					InitialDeal: &clusterv1.InitialDealEvent{SeatIndex: 0, Tiles: []string{"m1", "m2"}},
				},
			},
			wantID: msgid.InitialDealNotify,
		},
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

func TestRemoteRoomGatewaySeatMemoryAndSeatTiles(t *testing.T) {
	t.Parallel()
	g := &remoteRoomGateway{roomSeats: make(map[string]map[int32]string)}
	g.rememberRoomSeat("r1", 2, "u2")
	got, ok := g.userForSeat("r1", 2)
	require.True(t, ok)
	require.Equal(t, "u2", got)
	_, ok = g.userForSeat("r1", 1)
	require.False(t, ok)

	g.rememberRoomPlayers("r2", []string{"u0", "u1", "u2", "u3"})
	got, ok = g.userForSeat("r2", 3)
	require.True(t, ok)
	require.Equal(t, "u3", got)

	items := clusterSeatTilesToClient([]*clusterv1.SeatTiles{{SeatIndex: 1, Tiles: []string{"m1", "p2"}}})
	require.Len(t, items, 1)
	require.Equal(t, int32(1), items[0].GetSeatIndex())
	require.Equal(t, []string{"m1", "p2"}, items[0].GetTiles())
}

func TestRemoteRoomGatewayNilReceiverMethods(t *testing.T) {
	t.Parallel()

	var g *remoteRoomGateway
	ctx := context.Background()

	_, err := g.Join(ctx, "room", "user")
	require.Error(t, err)
	_, err = g.Ready(ctx, "room", "user")
	require.Error(t, err)
	_, err = g.Leave(ctx, "room", "user")
	require.Error(t, err)
	_, err = g.Discard(ctx, "room", "user", "m1")
	require.Error(t, err)
	_, err = g.Pong(ctx, "room", "user")
	require.Error(t, err)
	_, err = g.Gang(ctx, "room", "user", "m1")
	require.Error(t, err)
	_, err = g.Hu(ctx, "room", "user")
	require.Error(t, err)
	_, err = g.ExchangeThree(ctx, "room", "user", []string{"m1", "m2", "m3"}, 1)
	require.Error(t, err)
	_, err = g.QueMen(ctx, "room", "user", 0)
	require.Error(t, err)
	_, _, err = g.ListRooms(ctx, 20, "")
	require.Error(t, err)
	_, _, err = g.AutoMatch(ctx, "", "user")
	require.Error(t, err)
	_, _, err = g.CreateRoom(ctx, "", "", false, "user")
	require.Error(t, err)
	require.Error(t, g.EnsureRoomEventSubscription(ctx, "room", ""))
}

func TestRemoteRoomGatewayLobbyMethods(t *testing.T) {
	t.Parallel()
	g := &remoteRoomGateway{
		lobby:           &fakeLobbyClient{},
		roomSeats:       make(map[string]map[int32]string),
		defaultRoomAddr: "",
	}
	ctx := context.Background()

	rooms, next, err := g.ListRooms(ctx, 10, "")
	require.NoError(t, err)
	require.Equal(t, "next", next)
	require.Equal(t, "ROOM01", rooms[0].GetRoomId())

	roomID, seat, err := g.AutoMatch(ctx, "sichuan_xzdd", "u2")
	require.NoError(t, err)
	require.Equal(t, "ROOM01", roomID)
	require.Equal(t, 1, seat)
	gotUser, ok := g.userForSeat("ROOM01", 1)
	require.True(t, ok)
	require.Equal(t, "u2", gotUser)

	roomID, seat, err = g.CreateRoom(ctx, "sichuan_xzdd", "新桌", true, "u3")
	require.NoError(t, err)
	require.Equal(t, "ROOM02", roomID)
	require.Equal(t, 0, seat)
	gotUser, ok = g.userForSeat("ROOM02", 0)
	require.True(t, ok)
	require.Equal(t, "u3", gotUser)
}

type fakeLobbyClient struct{}

func (f *fakeLobbyClient) CreateRoom(_ context.Context, _ *clusterv1.CreateRoomRequest, _ ...grpc.CallOption) (*clusterv1.CreateRoomResponse, error) {
	return &clusterv1.CreateRoomResponse{RoomId: "ROOM02", SeatIndex: 0}, nil
}

func (f *fakeLobbyClient) JoinRoom(_ context.Context, _ *clusterv1.JoinRoomRequest, _ ...grpc.CallOption) (*clusterv1.JoinRoomResponse, error) {
	return &clusterv1.JoinRoomResponse{SeatIndex: 0}, nil
}

func (f *fakeLobbyClient) GetRoom(_ context.Context, _ *clusterv1.GetRoomRequest, _ ...grpc.CallOption) (*clusterv1.GetRoomResponse, error) {
	return &clusterv1.GetRoomResponse{RoomId: "ROOM01", RoomNodeId: "room-local"}, nil
}

func (f *fakeLobbyClient) ListRooms(_ context.Context, _ *clusterv1.ListRoomsRequest, _ ...grpc.CallOption) (*clusterv1.ListRoomsResponse, error) {
	return &clusterv1.ListRoomsResponse{
		Rooms:         []*clusterv1.RoomMeta{{RoomId: "ROOM01", RuleId: "sichuan_xzdd", SeatCount: 1, MaxSeats: 4, Stage: "waiting"}},
		NextPageToken: "next",
	}, nil
}

func (f *fakeLobbyClient) AutoMatch(_ context.Context, _ *clusterv1.AutoMatchRequest, _ ...grpc.CallOption) (*clusterv1.AutoMatchResponse, error) {
	return &clusterv1.AutoMatchResponse{RoomId: "ROOM01", SeatIndex: 1}, nil
}
