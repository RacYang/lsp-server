// remote room gateway 中纯函数与转换器的单元测试，避免起 grpc 依赖。
package remote

import (
	"context"
	"errors"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	svcv1 "racoo.cn/lsp/api/gen/go/v1"
	"racoo.cn/lsp/internal/net/msgid"
	"racoo.cn/lsp/internal/session"
	"racoo.cn/lsp/internal/store/redis"
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

// TestProtoTypesDirectPassthrough 验证 proto 统一后 client.v1 类型可以直接使用，无须 cluster 层中转。
// proto 统一前存在 clusterXxxToClient 系列转换函数；统一后两侧类型合并为 client.v1，转换函数已删除。
func TestProtoTypesDirectPassthrough(t *testing.T) {
	t.Parallel()

	// ClaimCandidate：直接构造 client.v1 类型，字段完整透传。
	candidates := []*clientv1.ClaimCandidate{{SeatIndex: 1, Actions: []string{"pong", "gang"}}}
	require.Equal(t, int32(1), candidates[0].GetSeatIndex())
	require.Equal(t, []string{"pong", "gang"}, candidates[0].GetActions())

	// SeatScore：TotalFan、Skipped 等字段直接可用。
	seatScores := []*clientv1.SeatScore{{SeatIndex: 2, UserId: "u", TotalFan: 10, Skipped: true}}
	require.Equal(t, "u", seatScores[0].GetUserId())
	require.Equal(t, int32(10), seatScores[0].GetTotalFan())
	require.True(t, seatScores[0].GetSkipped())

	// PenaltyItem：Amount 字段直接可用。
	penalties := []*clientv1.PenaltyItem{{Reason: "miss", FromSeat: 0, ToSeat: 1, Amount: 4}}
	require.Equal(t, int32(4), penalties[0].GetAmount())

	// WinnerBreakdown：FanNames 切片直接可用。
	breakdowns := []*clientv1.WinnerBreakdown{{SeatIndex: 3, UserId: "w", Fan: 6, FanNames: []string{"清一色"}}}
	require.Equal(t, []string{"清一色"}, breakdowns[0].GetFanNames())

	// RoomMeta：RoomId、SeatCount 等字段直接可用。
	rooms := []*clientv1.RoomMeta{{RoomId: "ROOM01", RuleId: "sichuan_xuezhandaodi_huansanzhang", DisplayName: "公开桌", SeatCount: 1, MaxSeats: 4, Stage: "waiting"}}
	require.Equal(t, "ROOM01", rooms[0].GetRoomId())
	require.Equal(t, int32(1), rooms[0].GetSeatCount())
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
		evt    *svcv1.RoomServiceStreamEventsResponse
		wantID uint16
	}{
		{
			name: "initial_deal",
			evt: &svcv1.RoomServiceStreamEventsResponse{
				Cursor: "deal-0",
				Body: &svcv1.RoomServiceStreamEventsResponse_InitialDeal{
					InitialDeal: &clientv1.InitialDealNotify{SeatIndex: 0, Tiles: []string{"m1", "m2"}},
				},
			},
			wantID: msgid.InitialDealNotify,
		},
		{
			name: "start_game",
			evt: &svcv1.RoomServiceStreamEventsResponse{
				RoomId: "r1",
				Cursor: "1",
				Body: &svcv1.RoomServiceStreamEventsResponse_StartGame{
					StartGame: &clientv1.StartGameNotify{DealerSeat: 2},
				},
			},
			wantID: msgid.StartGame,
		},
		{
			name: "draw_tile",
			evt: &svcv1.RoomServiceStreamEventsResponse{
				Body: &svcv1.RoomServiceStreamEventsResponse_DrawTile{
					DrawTile: &clientv1.DrawTileNotify{SeatIndex: 1, Tile: "1m"},
				},
			},
			wantID: msgid.DrawTile,
		},
		{
			name: "action",
			evt: &svcv1.RoomServiceStreamEventsResponse{
				Body: &svcv1.RoomServiceStreamEventsResponse_Action{
					Action: &clientv1.ActionNotify{SeatIndex: 0, Action: "pong", Tile: "5w"},
				},
			},
			wantID: msgid.ActionNotify,
		},
		{
			name: "settlement",
			evt: &svcv1.RoomServiceStreamEventsResponse{
				RoomId: "r-set",
				Body: &svcv1.RoomServiceStreamEventsResponse_Settlement{
					Settlement: &clientv1.SettlementNotify{
						WinnerUserIds: []string{"u1"},
						TotalFan:      4,
						SeatScores:    []*clientv1.SeatScore{{SeatIndex: 0}},
						Penalties:     []*clientv1.PenaltyItem{{Reason: "x"}},
					},
				},
			},
			wantID: msgid.Settlement,
		},
		{
			name: "opening_done_exchange",
			evt: &svcv1.RoomServiceStreamEventsResponse{
				Body: &svcv1.RoomServiceStreamEventsResponse_OpeningDone{
					OpeningDone: &clientv1.OpeningDoneNotify{
						Action:    "exchange_three",
						Kind:      "exchange_done",
						SeatTiles: []*clientv1.OpeningSeatTiles{{Key: "received", Seats: []*clientv1.SeatTiles{{SeatIndex: 0, Tiles: []string{"1m"}}}}},
					},
				},
			},
			wantID: msgid.OpeningDone,
		},
		{
			name: "opening_done_que",
			evt: &svcv1.RoomServiceStreamEventsResponse{
				Body: &svcv1.RoomServiceStreamEventsResponse_OpeningDone{
					OpeningDone: &clientv1.OpeningDoneNotify{
						Action:   "que_men",
						Kind:     "missing_suit_done",
						SeatInts: []*clientv1.OpeningSeatInts{{Key: "que_suit", Values: []int32{0, 1, 2, 0}}},
					},
				},
			},
			wantID: msgid.OpeningDone,
		},
		{
			name: "route_redirect",
			evt: &svcv1.RoomServiceStreamEventsResponse{
				Body: &svcv1.RoomServiceStreamEventsResponse_RouteRedirect{
					RouteRedirect: &clientv1.RouteRedirectNotify{WsUrl: "ws://x", Reason: "moved"},
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
		_, _, err := encodeClusterRoomEvent(&svcv1.RoomServiceStreamEventsResponse{})
		require.Error(t, err)
	})
}

func TestEncodeClusterRoomEventCarriesRoundProgress(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		evt    *svcv1.RoomServiceStreamEventsResponse
		assert func(*testing.T, *clientv1.Envelope)
	}{
		{
			name: "start",
			evt: &svcv1.RoomServiceStreamEventsResponse{
				RoomId: "r-progress",
				Cursor: "r-progress:10",
				Body: &svcv1.RoomServiceStreamEventsResponse_StartGame{
					StartGame: &clientv1.StartGameNotify{
						DealerSeat:    0,
						Phase:         clientv1.Phase_PHASE_DRAW,
						Step:          10,
						ActingSeats:   []int32{0},
						WallRemaining: 55,
					},
				},
			},
			assert: func(t *testing.T, env *clientv1.Envelope) {
				t.Helper()
				require.Equal(t, clientv1.Phase_PHASE_DRAW, env.GetStartGame().GetPhase())
				require.EqualValues(t, 10, env.GetStartGame().GetStep())
				require.Equal(t, []int32{0}, env.GetStartGame().GetActingSeats())
				require.EqualValues(t, 55, env.GetStartGame().GetWallRemaining())
			},
		},
		{
			name: "draw",
			evt: &svcv1.RoomServiceStreamEventsResponse{
				RoomId: "r-progress",
				Cursor: "r-progress:11",
				Body: &svcv1.RoomServiceStreamEventsResponse_DrawTile{
					DrawTile: &clientv1.DrawTileNotify{
						SeatIndex:      1,
						Tile:           "p9",
						Phase:          clientv1.Phase_PHASE_DISCARD,
						Step:           11,
						ActingSeats:    []int32{1},
						WallRemaining:  54,
						DeadlineUnixMs: 1234,
					},
				},
			},
			assert: func(t *testing.T, env *clientv1.Envelope) {
				t.Helper()
				require.Equal(t, clientv1.Phase_PHASE_DISCARD, env.GetDrawTile().GetPhase())
				require.EqualValues(t, 11, env.GetDrawTile().GetStep())
				require.Equal(t, []int32{1}, env.GetDrawTile().GetActingSeats())
				require.EqualValues(t, 54, env.GetDrawTile().GetWallRemaining())
				require.EqualValues(t, 1234, env.GetDrawTile().GetDeadlineUnixMs())
			},
		},
		{
			name: "action",
			evt: &svcv1.RoomServiceStreamEventsResponse{
				RoomId: "r-progress",
				Cursor: "r-progress:12",
				Body: &svcv1.RoomServiceStreamEventsResponse_Action{
					Action: &clientv1.ActionNotify{
						SeatIndex:   1,
						Action:      "discard",
						Tile:        "p9",
						Phase:       clientv1.Phase_PHASE_DRAW,
						Step:        12,
						ActingSeats: []int32{2},
					},
				},
			},
			assert: func(t *testing.T, env *clientv1.Envelope) {
				t.Helper()
				require.Equal(t, clientv1.Phase_PHASE_DRAW, env.GetAction().GetPhase())
				require.EqualValues(t, 12, env.GetAction().GetStep())
				require.Equal(t, []int32{2}, env.GetAction().GetActingSeats())
			},
		},
		{
			name: "exchange",
			evt: &svcv1.RoomServiceStreamEventsResponse{
				RoomId: "r-progress",
				Cursor: "r-progress:13",
				Body: &svcv1.RoomServiceStreamEventsResponse_OpeningDone{
					OpeningDone: &clientv1.OpeningDoneNotify{
						Action:      "exchange_three",
						Kind:        "exchange_done",
						Params:      map[string]string{"direction": "3"},
						Phase:       clientv1.Phase_PHASE_OPENING,
						Step:        13,
						ActingSeats: []int32{0, 1, 2, 3},
					},
				},
			},
			assert: func(t *testing.T, env *clientv1.Envelope) {
				t.Helper()
				require.Equal(t, clientv1.Phase_PHASE_OPENING, env.GetOpeningDone().GetPhase())
				require.EqualValues(t, 13, env.GetOpeningDone().GetStep())
				require.Equal(t, []int32{0, 1, 2, 3}, env.GetOpeningDone().GetActingSeats())
				require.Equal(t, "3", env.GetOpeningDone().GetParams()["direction"])
			},
		},
		{
			name: "que",
			evt: &svcv1.RoomServiceStreamEventsResponse{
				RoomId: "r-progress",
				Cursor: "r-progress:14",
				Body: &svcv1.RoomServiceStreamEventsResponse_OpeningDone{
					OpeningDone: &clientv1.OpeningDoneNotify{
						Action:      "que_men",
						Kind:        "missing_suit_done",
						Phase:       clientv1.Phase_PHASE_DRAW,
						Step:        14,
						ActingSeats: []int32{0},
					},
				},
			},
			assert: func(t *testing.T, env *clientv1.Envelope) {
				t.Helper()
				require.Equal(t, clientv1.Phase_PHASE_DRAW, env.GetOpeningDone().GetPhase())
				require.EqualValues(t, 14, env.GetOpeningDone().GetStep())
				require.Equal(t, []int32{0}, env.GetOpeningDone().GetActingSeats())
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, payload, err := encodeClusterRoomEvent(tc.evt)
			require.NoError(t, err)

			var env clientv1.Envelope
			require.NoError(t, proto.Unmarshal(payload, &env))
			tc.assert(t, &env)
		})
	}
}

// TestClientSeatInfoHandCount 验证 proto 统一后 client.v1.SeatInfo 直接携带 HandCount 字段，无须转换。
func TestClientSeatInfoHandCount(t *testing.T) {
	t.Parallel()

	// proto 统一后 cluster/v1 不再有独立的 SeatInfo，直接使用 client.v1.SeatInfo。
	seats := []*clientv1.SeatInfo{{
		SeatIndex: 1,
		UserId:    "bot:r1:1",
		Status:    "ready",
		HandCount: 13,
	}}

	require.Len(t, seats, 1)
	require.EqualValues(t, 13, seats[0].GetHandCount())
	require.Equal(t, "ready", seats[0].GetStatus())
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
	g.rememberRoomPlayers("r2", []string{"u0-new", "u1-new"})
	got, ok = g.userForSeat("r2", 0)
	require.True(t, ok)
	require.Equal(t, "u0-new", got)
	_, ok = g.userForSeat("r2", 3)
	require.False(t, ok, "快照刷新座位映射时必须清掉已经离座的旧用户")
	g.rememberRoomPlayers("r2", nil)
	got, ok = g.userForSeat("r2", 0)
	require.True(t, ok)
	require.Equal(t, "u0-new", got, "旧版空 player_ids 快照不能清空已有定向映射")
	g.rememberRoomSeatInfos("r2", []*clientv1.SeatInfo{{SeatIndex: 2, UserId: "u2-seat"}})
	got, ok = g.userForSeat("r2", 2)
	require.True(t, ok)
	require.Equal(t, "u2-seat", got)
	got, ok = g.userForSeat("r2", 0)
	require.True(t, ok)
	require.Equal(t, "u0-new", got, "room 进程部分快照不能清掉 lobby 已知座位")
	g.rememberRoomSeatInfos("r2", []*clientv1.SeatInfo{
		{SeatIndex: 1, UserId: "u1-final"},
		{SeatIndex: 2, UserId: "u2-final"},
		{SeatIndex: 3, UserId: "u3-final"},
	})
	_, ok = g.userForSeat("r2", 0)
	require.False(t, ok, "完整度更高的 seats 快照应清理旧座位")
	got, ok = g.userForSeat("r2", 3)
	require.True(t, ok)
	require.Equal(t, "u3-final", got)

	// proto 统一后 SeatTiles 直接使用 client.v1 类型，无须 clusterSeatTilesToClient 转换。
	items := []*clientv1.SeatTiles{{SeatIndex: 1, Tiles: []string{"m1", "p2"}}}
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
	_, err = g.Discard(ctx, "room", "user", "m1", nil)
	require.Error(t, err)
	_, err = g.Pong(ctx, "room", "user", nil)
	require.Error(t, err)
	_, err = g.Gang(ctx, "room", "user", "m1", nil)
	require.Error(t, err)
	_, err = g.Hu(ctx, "room", "user", nil)
	require.Error(t, err)
	_, err = g.OpeningAction(ctx, "room", "user", "exchange_three", []string{"m1", "m2", "m3"}, 1, 0, nil, nil)
	require.Error(t, err)
	_, err = g.OpeningAction(ctx, "room", "user", "que_men", nil, 0, 0, nil, nil)
	require.Error(t, err)
	_, _, err = g.ListRooms(ctx, 20, "")
	require.Error(t, err)
	_, _, err = g.AutoMatch(ctx, "", "user", false)
	require.Error(t, err)
	_, _, err = g.CreateRoom(ctx, "", "", false, "user")
	require.Error(t, err)
	require.Error(t, g.EnsureRoomEventSubscription(ctx, "room", ""))
}

func TestRemoteRoomGatewayResumeLobbySessionIgnoresLegacyAdvertise(t *testing.T) {
	t.Parallel()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)
	rcli := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rcli.Close() })
	store := redis.NewClientFromUniversal(rcli)
	mgr := session.NewManager(store)
	ctx := context.Background()

	tok, err := mgr.Issue(ctx, "lobby-user")
	require.NoError(t, err)
	rec, ok, err := store.GetSession(ctx, "lobby-user")
	require.NoError(t, err)
	require.True(t, ok)
	rec.AdvertiseAddr = "legacy-gate.example"
	rec.GateNodeID = "legacy-gate"
	require.NoError(t, store.PutSession(ctx, "lobby-user", rec, time.Minute))

	g := &remoteRoomGateway{sess: mgr}
	res, err := g.Resume(ctx, tok)
	require.NoError(t, err)
	require.Equal(t, "lobby-user", res.UserID)
	require.Empty(t, res.RoomID)
	require.False(t, res.Resumed)
	require.Nil(t, res.Redirect)
}

func TestRemoteRoomGatewayLobbyMethods(t *testing.T) {
	t.Parallel()
	g := &remoteRoomGateway{
		lobby:           &fakeLobbyClient{},
		roomSeats:       make(map[string]map[int32]string),
		defaultRoomAddr: "room-local",
		roomClients:     map[string]svcv1.RoomServiceClient{"room-local": &fakeRoomClient{state: "waiting"}},
		pollCtx:         context.Background(),
		pollHandles:     make(map[string]context.CancelFunc),
	}
	ctx := context.Background()

	rooms, next, err := g.ListRooms(ctx, 10, "")
	require.NoError(t, err)
	require.Equal(t, "next", next)
	require.Equal(t, "ROOM01", rooms[0].GetRoomId())

	roomID, seat, err := g.AutoMatch(ctx, "sichuan_xuezhandaodi_huansanzhang", "u2", false)
	require.NoError(t, err)
	require.Equal(t, "ROOM01", roomID)
	require.Equal(t, 1, seat)
	gotUser, ok := g.userForSeat("ROOM01", 1)
	require.True(t, ok)
	require.Equal(t, "u2", gotUser)

	roomID, seat, err = g.CreateRoom(ctx, "sichuan_xuezhandaodi_huansanzhang", "新桌", true, "u3")
	require.NoError(t, err)
	require.Equal(t, "ROOM02", roomID)
	require.Equal(t, 0, seat)
	gotUser, ok = g.userForSeat("ROOM02", 0)
	require.True(t, ok)
	require.Equal(t, "u3", gotUser)
}

func TestRemoteRoomGatewayAutoMatchSkipsStartedRoom(t *testing.T) {
	t.Parallel()
	lobby := &fakeAutoMatchLobby{}
	g := &remoteRoomGateway{
		lobby:           lobby,
		defaultRoomAddr: "room-local",
		roomClients:     map[string]svcv1.RoomServiceClient{"room-local": &fakeRoomClient{state: "playing"}},
		roomSeats:       make(map[string]map[int32]string),
		pollCtx:         context.Background(),
		pollHandles:     make(map[string]context.CancelFunc),
	}

	roomID, seat, err := g.AutoMatch(context.Background(), "sichuan_xuezhandaodi_huansanzhang", "u2", false)
	require.NoError(t, err)
	require.Equal(t, "ROOM02", roomID)
	require.Equal(t, 0, seat)
	require.Equal(t, 1, lobby.left)
}

type fakeLobbyClient struct{}

type fakeAutoMatchLobby struct {
	left int
}

func (f *fakeAutoMatchLobby) CreateRoom(_ context.Context, _ *svcv1.CreateRoomRequest, _ ...grpc.CallOption) (*svcv1.CreateRoomResponse, error) {
	return &svcv1.CreateRoomResponse{RoomId: "ROOM02", SeatIndex: 0}, nil
}

func (f *fakeAutoMatchLobby) JoinRoom(_ context.Context, req *svcv1.JoinRoomRequest, _ ...grpc.CallOption) (*svcv1.JoinRoomResponse, error) {
	if req.GetRoomId() == "ROOM01" {
		return &svcv1.JoinRoomResponse{SeatIndex: 1}, nil
	}
	return &svcv1.JoinRoomResponse{Error: "room not found"}, nil
}

func (f *fakeAutoMatchLobby) GetRoom(_ context.Context, _ *svcv1.GetRoomRequest, _ ...grpc.CallOption) (*svcv1.GetRoomResponse, error) {
	return &svcv1.GetRoomResponse{RoomId: "ROOM01", RoomNodeId: "room-local"}, nil
}

func (f *fakeAutoMatchLobby) ListRooms(_ context.Context, _ *svcv1.ListRoomsRequest, _ ...grpc.CallOption) (*svcv1.ListRoomsResponse, error) {
	return &svcv1.ListRoomsResponse{Rooms: []*clientv1.RoomMeta{{RoomId: "ROOM01", RuleId: "sichuan_xuezhandaodi_huansanzhang", SeatCount: 1, MaxSeats: 4, Stage: "waiting"}}}, nil
}

func (f *fakeAutoMatchLobby) ListRules(_ context.Context, _ *svcv1.ListRulesRequest, _ ...grpc.CallOption) (*svcv1.ListRulesResponse, error) {
	return &svcv1.ListRulesResponse{}, nil
}

func (f *fakeAutoMatchLobby) AutoMatch(_ context.Context, _ *svcv1.AutoMatchRequest, _ ...grpc.CallOption) (*svcv1.AutoMatchResponse, error) {
	return &svcv1.AutoMatchResponse{}, nil
}

func (f *fakeAutoMatchLobby) AddBot(_ context.Context, _ *svcv1.AddBotRequest, _ ...grpc.CallOption) (*svcv1.AddBotResponse, error) {
	return &svcv1.AddBotResponse{}, nil
}

func (f *fakeAutoMatchLobby) LeaveRoom(_ context.Context, _ *svcv1.LeaveRoomRequest, _ ...grpc.CallOption) (*svcv1.LeaveRoomResponse, error) {
	f.left++
	return &svcv1.LeaveRoomResponse{}, nil
}

type fakeRoomClient struct {
	state string
}

func (f *fakeRoomClient) ApplyEvent(_ context.Context, _ *svcv1.ApplyEventRequest, _ ...grpc.CallOption) (*svcv1.ApplyEventResponse, error) {
	return &svcv1.ApplyEventResponse{Accepted: true}, nil
}

func (f *fakeRoomClient) StreamEvents(_ context.Context, _ *svcv1.StreamEventsRequest, _ ...grpc.CallOption) (grpc.ServerStreamingClient[svcv1.RoomServiceStreamEventsResponse], error) {
	return nil, errors.New("stream not implemented")
}

func (f *fakeRoomClient) SnapshotRoom(_ context.Context, _ *svcv1.SnapshotRoomRequest, _ ...grpc.CallOption) (*svcv1.SnapshotRoomResponse, error) {
	return &svcv1.SnapshotRoomResponse{State: f.state}, nil
}

func (f *fakeRoomClient) GetRoomEvents(_ context.Context, _ *svcv1.GetRoomEventsRequest, _ ...grpc.CallOption) (*svcv1.GetRoomEventsResponse, error) {
	return &svcv1.GetRoomEventsResponse{}, nil
}

func (f *fakeLobbyClient) CreateRoom(_ context.Context, _ *svcv1.CreateRoomRequest, _ ...grpc.CallOption) (*svcv1.CreateRoomResponse, error) {
	return &svcv1.CreateRoomResponse{RoomId: "ROOM02", SeatIndex: 0}, nil
}

func (f *fakeLobbyClient) JoinRoom(_ context.Context, _ *svcv1.JoinRoomRequest, _ ...grpc.CallOption) (*svcv1.JoinRoomResponse, error) {
	return &svcv1.JoinRoomResponse{SeatIndex: 1}, nil
}

func (f *fakeLobbyClient) GetRoom(_ context.Context, _ *svcv1.GetRoomRequest, _ ...grpc.CallOption) (*svcv1.GetRoomResponse, error) {
	return &svcv1.GetRoomResponse{RoomId: "ROOM01", RoomNodeId: "room-local"}, nil
}

func (f *fakeLobbyClient) ListRooms(_ context.Context, _ *svcv1.ListRoomsRequest, _ ...grpc.CallOption) (*svcv1.ListRoomsResponse, error) {
	return &svcv1.ListRoomsResponse{
		Rooms:         []*clientv1.RoomMeta{{RoomId: "ROOM01", RuleId: "sichuan_xuezhandaodi_huansanzhang", SeatCount: 1, MaxSeats: 4, Stage: "waiting"}},
		NextPageToken: "next",
	}, nil
}

func (f *fakeLobbyClient) ListRules(_ context.Context, _ *svcv1.ListRulesRequest, _ ...grpc.CallOption) (*svcv1.ListRulesResponse, error) {
	return &svcv1.ListRulesResponse{Rules: []*clientv1.RuleMeta{{RuleId: "sichuan_xuezhandaodi_huansanzhang"}}}, nil
}

func (f *fakeLobbyClient) AutoMatch(_ context.Context, _ *svcv1.AutoMatchRequest, _ ...grpc.CallOption) (*svcv1.AutoMatchResponse, error) {
	return &svcv1.AutoMatchResponse{RoomId: "ROOM01", SeatIndex: 1}, nil
}

func (f *fakeLobbyClient) AddBot(_ context.Context, _ *svcv1.AddBotRequest, _ ...grpc.CallOption) (*svcv1.AddBotResponse, error) {
	return &svcv1.AddBotResponse{}, nil
}

func (f *fakeLobbyClient) LeaveRoom(_ context.Context, _ *svcv1.LeaveRoomRequest, _ ...grpc.CallOption) (*svcv1.LeaveRoomResponse, error) {
	return &svcv1.LeaveRoomResponse{}, nil
}
