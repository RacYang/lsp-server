package roomadapter

import (
	"context"
	"net"
	"testing"
	"time"

	eng "racoo.cn/lsp/internal/service/room/engine"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v3"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	svcv1 "racoo.cn/lsp/api/gen/go/v1"
	roomsvc "racoo.cn/lsp/internal/service/room"
	"racoo.cn/lsp/internal/store/postgres"
	"racoo.cn/lsp/internal/store/redis"

	// 触发麻将规则注册。
	_ "racoo.cn/lsp/internal/mahjong/sichuan/xuezhandaodi"
)

func TestRoomGRPCServerApplyEventAndStream(t *testing.T) {
	t.Parallel()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)
	rcli := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rcli.Close() })
	rdb := redis.NewClientFromUniversal(rcli)

	var lc net.ListenConfig
	ln, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = ln.Close() }()

	grpcSrv := grpc.NewServer()
	srv := NewGRPCServer(roomsvc.NewServiceWithRule(roomsvc.NewRoomRegistry(), "sichuan_xuezhandaodi_huansanzhang"), nil, nil, nil, rdb)
	RegisterService(grpcSrv, srv)
	go func() { _ = grpcSrv.Serve(ln) }()
	defer grpcSrv.Stop()

	conn, err := grpc.NewClient(ln.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	client := svcv1.NewRoomServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 四位玩家依次加入并准备，准备完毕后服务端自动开局并将事件推送至 Redis 队列。
	for _, userID := range []string{"u1", "u2", "u3", "u4"} {
		resp, err := client.ApplyEvent(ctx, &svcv1.ApplyEventRequest{
			RoomId: "r1",
			UserId: userID,
			Body:   &svcv1.ApplyEventRequest_Ready{Ready: &svcv1.ReadyEvent{}},
		})
		require.NoError(t, err)
		require.True(t, resp.GetAccepted())
	}

	// 从 Redis 队列轮询事件并驱动游戏直至结算。
	var gotSettlement bool
	players := []string{"u1", "u2", "u3", "u4"}
	hands := make([][]string, 4)
	for i := 0; i < 512; i++ {
		data, popErr := rdb.RoomEventQueuePop(ctx, "r1", 200*time.Millisecond)
		if popErr != nil {
			require.NoError(t, popErr)
		}
		if data == nil {
			continue // 超时，继续轮询
		}
		var evt svcv1.RoomServiceStreamEventsResponse
		require.NoError(t, proto.Unmarshal(data, &evt))
		require.Equal(t, "r1", evt.GetRoomId())
		if deal := evt.GetInitialDeal(); deal != nil {
			hands[deal.GetSeatIndex()] = append([]string(nil), deal.GetTiles()...)
		}
		if draw := evt.GetDrawTile(); draw != nil {
			if draw.GetTile() == "" {
				continue
			}
			_, err = client.ApplyEvent(ctx, &svcv1.ApplyEventRequest{
				RoomId: "r1",
				UserId: players[draw.GetSeatIndex()],
				Body:   &svcv1.ApplyEventRequest_Discard{Discard: &svcv1.DiscardEvent{Tile: draw.GetTile()}},
			})
			require.NoError(t, err)
		}
		if action := evt.GetAction(); action != nil {
			switch action.GetAction() {
			case "exchange_three":
				seat := action.GetSeatIndex()
				require.GreaterOrEqual(t, len(hands[seat]), 3)
				_, err = client.ApplyEvent(ctx, &svcv1.ApplyEventRequest{
					RoomId: "r1",
					UserId: players[action.GetSeatIndex()],
					Body: &svcv1.ApplyEventRequest_OpeningAction{OpeningAction: &svcv1.OpeningActionEvent{
						Action: "exchange_three",
						Tiles:  append([]string(nil), hands[seat][:3]...),
					}},
				})
				require.NoError(t, err)
			case "que_men":
				_, err = client.ApplyEvent(ctx, &svcv1.ApplyEventRequest{
					RoomId: "r1",
					UserId: players[action.GetSeatIndex()],
					Body: &svcv1.ApplyEventRequest_OpeningAction{OpeningAction: &svcv1.OpeningActionEvent{
						Action: "que_men",
						Suit:   0,
					}},
				})
				require.NoError(t, err)
			case "pong_choice":
				_, err = client.ApplyEvent(ctx, &svcv1.ApplyEventRequest{
					RoomId: "r1",
					UserId: players[action.GetSeatIndex()],
					Body:   &svcv1.ApplyEventRequest_Pass{Pass: &svcv1.PassEvent{}},
				})
				require.NoError(t, err)
			case "gang_choice":
				_, err = client.ApplyEvent(ctx, &svcv1.ApplyEventRequest{
					RoomId: "r1",
					UserId: players[action.GetSeatIndex()],
					Body:   &svcv1.ApplyEventRequest_Gang{Gang: &svcv1.GangEvent{Tile: action.GetTile()}},
				})
				require.NoError(t, err)
			case "hu_choice", "qiang_gang_choice", "tsumo_choice":
				_, err = client.ApplyEvent(ctx, &svcv1.ApplyEventRequest{
					RoomId: "r1",
					UserId: players[action.GetSeatIndex()],
					Body:   &svcv1.ApplyEventRequest_Hu{Hu: &svcv1.HuEvent{}},
				})
				require.NoError(t, err)
			}
		}
		if evt.GetSettlement() != nil {
			gotSettlement = true
			break
		}
	}
	require.True(t, gotSettlement)
}

func TestApplyEventIdempotencyRetryAfterFailure(t *testing.T) {
	t.Parallel()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)
	rcli := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rcli.Close() })
	rdb := redis.NewClientFromUniversal(rcli)

	s := NewGRPCServer(roomsvc.NewServiceWithRule(roomsvc.NewRoomRegistry(), "sichuan_xuezhandaodi_huansanzhang"), nil, nil, nil, rdb)
	ctx := context.Background()

	s.SetReady(false)
	resp1, err := s.ApplyEvent(ctx, &svcv1.ApplyEventRequest{
		RoomId:         "r-idem",
		UserId:         "u1",
		IdempotencyKey: "k-retry",
		Body:           &svcv1.ApplyEventRequest_Ready{Ready: &svcv1.ReadyEvent{}},
	})
	require.NoError(t, err)
	require.False(t, resp1.GetAccepted())

	s.SetReady(true)
	resp2, err := s.ApplyEvent(ctx, &svcv1.ApplyEventRequest{
		RoomId:         "r-idem",
		UserId:         "u1",
		IdempotencyKey: "k-retry",
		Body:           &svcv1.ApplyEventRequest_Ready{Ready: &svcv1.ReadyEvent{}},
	})
	require.NoError(t, err)
	require.True(t, resp2.GetAccepted())

	resp3, err := s.ApplyEvent(ctx, &svcv1.ApplyEventRequest{
		RoomId:         "r-idem",
		UserId:         "u1",
		IdempotencyKey: "k-retry",
		Body:           &svcv1.ApplyEventRequest_Ready{Ready: &svcv1.ReadyEvent{}},
	})
	require.NoError(t, err)
	require.True(t, resp3.GetAccepted())
}

func TestSnapshotRoomIncludesRoundView(t *testing.T) {
	t.Parallel()

	srv := NewGRPCServer(roomsvc.NewServiceWithRule(roomsvc.NewRoomRegistry(), "sichuan_xuezhandaodi_huansanzhang"), nil, nil, nil, nil)
	ctx := context.Background()
	for _, userID := range []string{"u1", "u2", "u3", "u4"} {
		_, err := srv.ApplyEvent(ctx, &svcv1.ApplyEventRequest{
			RoomId: "r-snap",
			UserId: userID,
			Body:   &svcv1.ApplyEventRequest_Ready{Ready: &svcv1.ReadyEvent{}},
		})
		require.NoError(t, err)
	}

	snap, err := srv.SnapshotRoom(ctx, &svcv1.SnapshotRoomRequest{RoomId: "r-snap"})
	require.NoError(t, err)
	require.Equal(t, "playing", snap.GetState())
	require.EqualValues(t, 0, snap.GetActingSeat())
	require.Equal(t, "exchange_three", snap.GetWaitingAction())
	require.Contains(t, snap.GetAvailableActions(), "exchange_three")
}

// TestProtoTypesDirectUsageInRoom 验证 proto 统一后，结算相关消息类型可在 room 侧直接构造与读取，
// 不再需要 cluster 层的冗余转换函数。
func TestProtoTypesDirectUsageInRoom(t *testing.T) {
	t.Parallel()

	// 座位分数：直接构造客户端类型，字段完整可读。
	scores := []*clientv1.SeatScore{{SeatIndex: 1, UserId: "u1", TotalFan: 8, Skipped: true}}
	require.Equal(t, "u1", scores[0].GetUserId())
	require.EqualValues(t, 8, scores[0].GetTotalFan())
	require.True(t, scores[0].GetSkipped())

	// 罚分条目：直接构造客户端类型，字段完整可读。
	penalties := []*clientv1.PenaltyItem{{Reason: "查大叫", FromSeat: 0, ToSeat: 2, Amount: 16}}
	require.Equal(t, "查大叫", penalties[0].GetReason())
	require.EqualValues(t, 16, penalties[0].GetAmount())
}

func TestClusterPhaseTokToRoom(t *testing.T) {
	t.Parallel()

	tests := []struct {
		desc       string
		input      *clientv1.PhaseToken
		wantNil    bool
		wantReason eng.WaitingReason
		wantStep   int64
	}{
		{desc: "nil 透传 nil", input: nil, wantNil: true},
		{
			desc:       "开局阶段",
			input:      &clientv1.PhaseToken{Step: 1, Reason: clientv1.WaitingReason_WAITING_REASON_OPENING},
			wantReason: eng.ReasonOpening,
			wantStep:   1,
		},
		{
			desc:       "抢杠窗口",
			input:      &clientv1.PhaseToken{Step: 2, Reason: clientv1.WaitingReason_WAITING_REASON_CLAIM_WINDOW},
			wantReason: eng.ReasonClaimWindow,
			wantStep:   2,
		},
		{
			desc:       "摸牌自摸",
			input:      &clientv1.PhaseToken{Step: 3, Reason: clientv1.WaitingReason_WAITING_REASON_TSUMO},
			wantReason: eng.ReasonTsumo,
			wantStep:   3,
		},
		{
			desc:       "弃牌阶段",
			input:      &clientv1.PhaseToken{Step: 4, Reason: clientv1.WaitingReason_WAITING_REASON_DISCARD},
			wantReason: eng.ReasonDiscard,
			wantStep:   4,
		},
		{
			desc:       "投降阶段",
			input:      &clientv1.PhaseToken{Step: 5, Reason: clientv1.WaitingReason_WAITING_REASON_SURRENDER},
			wantReason: eng.ReasonSurrender,
			wantStep:   5,
		},
		{
			desc:       "未知原因回退 None",
			input:      &clientv1.PhaseToken{Step: 6, Reason: clientv1.WaitingReason_WAITING_REASON_NONE},
			wantReason: eng.ReasonNone,
			wantStep:   6,
		},
	}

	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			t.Parallel()
			got := eng.PhaseTokenFromProto(tc.input)
			if tc.wantNil {
				require.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			require.Equal(t, tc.wantReason, got.Reason)
			require.Equal(t, tc.wantStep, got.Step)
		})
	}
}

func TestWaitingReasonFromRoundView(t *testing.T) {
	t.Parallel()

	tests := []struct {
		desc string
		view eng.RoundView
		want clientv1.WaitingReason
	}{
		{
			desc: "开局阶段映射",
			view: eng.RoundView{Phase: eng.PhaseOpening},
			want: clientv1.WaitingReason_WAITING_REASON_OPENING,
		},
		{
			desc: "吃碰杠抢窗口映射",
			view: eng.RoundView{Phase: eng.PhaseClaim, WaitingAction: "claim_window"},
			want: clientv1.WaitingReason_WAITING_REASON_CLAIM_WINDOW,
		},
		{
			desc: "自摸窗口映射",
			view: eng.RoundView{Phase: eng.PhaseTsumo, WaitingAction: "tsumo_window"},
			want: clientv1.WaitingReason_WAITING_REASON_TSUMO,
		},
		{
			desc: "弃牌映射",
			view: eng.RoundView{Phase: eng.PhaseDiscard, WaitingAction: "discard"},
			want: clientv1.WaitingReason_WAITING_REASON_DISCARD,
		},
		{
			desc: "其它动作回退 None",
			view: eng.RoundView{Phase: eng.PhaseDraw, WaitingAction: "unknown"},
			want: clientv1.WaitingReason_WAITING_REASON_NONE,
		},
	}

	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			t.Parallel()
			got := waitingReasonFromRoundView(tc.view)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestMapPGRowToEvent(t *testing.T) {
	t.Parallel()

	payload, err := proto.Marshal(&clientv1.Envelope{
		Body: &clientv1.Envelope_StartGame{StartGame: &clientv1.StartGameNotify{DealerSeat: 2}},
	})
	require.NoError(t, err)

	evt, err := mapPGRowToEvent("r-pg", postgres.RoomEventRow{Seq: 7, Kind: string(eng.KindStartGame), Payload: payload})
	require.NoError(t, err)
	require.Equal(t, "r-pg:7", evt.GetCursor())
	require.EqualValues(t, 2, evt.GetStartGame().GetDealerSeat())
}

func TestMapNotificationToEventCarriesRoundProgress(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		kind   eng.Kind
		env    *clientv1.Envelope
		assert func(*testing.T, *svcv1.RoomServiceStreamEventsResponse)
	}{
		{
			name: "start",
			kind: eng.KindStartGame,
			env: &clientv1.Envelope{Body: &clientv1.Envelope_StartGame{StartGame: &clientv1.StartGameNotify{
				Phase:         eng.PhaseDraw.Proto(),
				Step:          10,
				ActingSeats:   []int32{0},
				WallRemaining: 55,
			}}},
			assert: func(t *testing.T, evt *svcv1.RoomServiceStreamEventsResponse) {
				t.Helper()
				require.Equal(t, eng.PhaseDraw.Proto(), evt.GetStartGame().GetPhase())
				require.EqualValues(t, 10, evt.GetStartGame().GetStep())
				require.Equal(t, []int32{0}, evt.GetStartGame().GetActingSeats())
				require.EqualValues(t, 55, evt.GetStartGame().GetWallRemaining())
			},
		},
		{
			name: "draw",
			kind: eng.KindDrawTile,
			env: &clientv1.Envelope{Body: &clientv1.Envelope_DrawTile{DrawTile: &clientv1.DrawTileNotify{
				Phase:          eng.PhaseDiscard.Proto(),
				Step:           11,
				ActingSeats:    []int32{1},
				WallRemaining:  54,
				DeadlineUnixMs: 1234,
			}}},
			assert: func(t *testing.T, evt *svcv1.RoomServiceStreamEventsResponse) {
				t.Helper()
				require.Equal(t, eng.PhaseDiscard.Proto(), evt.GetDrawTile().GetPhase())
				require.EqualValues(t, 11, evt.GetDrawTile().GetStep())
				require.Equal(t, []int32{1}, evt.GetDrawTile().GetActingSeats())
				require.EqualValues(t, 54, evt.GetDrawTile().GetWallRemaining())
				require.EqualValues(t, 1234, evt.GetDrawTile().GetDeadlineUnixMs())
			},
		},
		{
			name: "action",
			kind: eng.KindAction,
			env: &clientv1.Envelope{Body: &clientv1.Envelope_Action{Action: &clientv1.ActionNotify{
				SeatIndex:   1,
				Action:      "discard",
				Tile:        "p9",
				Phase:       eng.PhaseDraw.Proto(),
				Step:        12,
				ActingSeats: []int32{2},
			}}},
			assert: func(t *testing.T, evt *svcv1.RoomServiceStreamEventsResponse) {
				t.Helper()
				require.Equal(t, eng.PhaseDraw.Proto(), evt.GetAction().GetPhase())
				require.EqualValues(t, 12, evt.GetAction().GetStep())
				require.Equal(t, []int32{2}, evt.GetAction().GetActingSeats())
			},
		},
		{
			name: "exchange",
			kind: eng.KindOpeningDone,
			env: &clientv1.Envelope{Body: &clientv1.Envelope_OpeningDone{OpeningDone: &clientv1.OpeningDoneNotify{
				Action:      "exchange_three",
				Kind:        "exchange_done",
				Params:      map[string]string{"direction": "3"},
				Phase:       eng.PhaseOpening.Proto(),
				Step:        13,
				ActingSeats: []int32{0, 1, 2, 3},
			}}},
			assert: func(t *testing.T, evt *svcv1.RoomServiceStreamEventsResponse) {
				t.Helper()
				require.Equal(t, eng.PhaseOpening.Proto(), evt.GetOpeningDone().GetPhase())
				require.EqualValues(t, 13, evt.GetOpeningDone().GetStep())
				require.Equal(t, []int32{0, 1, 2, 3}, evt.GetOpeningDone().GetActingSeats())
				require.Equal(t, "3", evt.GetOpeningDone().GetParams()["direction"])
			},
		},
		{
			name: "que",
			kind: eng.KindOpeningDone,
			env: &clientv1.Envelope{Body: &clientv1.Envelope_OpeningDone{OpeningDone: &clientv1.OpeningDoneNotify{
				Action:      "que_men",
				Kind:        "missing_suit_done",
				Phase:       eng.PhaseDraw.Proto(),
				Step:        14,
				ActingSeats: []int32{0},
			}}},
			assert: func(t *testing.T, evt *svcv1.RoomServiceStreamEventsResponse) {
				t.Helper()
				require.Equal(t, eng.PhaseDraw.Proto(), evt.GetOpeningDone().GetPhase())
				require.EqualValues(t, 14, evt.GetOpeningDone().GetStep())
				require.Equal(t, []int32{0}, evt.GetOpeningDone().GetActingSeats())
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload, err := proto.Marshal(tc.env)
			require.NoError(t, err)

			evt, err := mapNotificationToEvent("r-progress", "r-progress:12", eng.Notification{
				Kind:       tc.kind,
				Payload:    payload,
				TargetSeat: eng.BroadcastSeat,
			})
			require.NoError(t, err)
			tc.assert(t, evt)
		})
	}
}

func TestPersistRoomMetaKeepsQueSuits(t *testing.T) {
	t.Parallel()

	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)
	rcli := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rcli.Close() })
	rdb := redis.NewClientFromUniversal(rcli)

	rooms := roomsvc.NewServiceWithRule(roomsvc.NewRoomRegistry(), "sichuan_xuezhandaodi_huansanzhang")
	for _, uid := range []string{"u1", "u2", "u3", "u4"} {
		_, err := rooms.Join(context.Background(), "r-meta", uid)
		require.NoError(t, err)
	}
	srv := NewGRPCServer(rooms, nil, nil, nil, rdb)
	ctx := context.Background()

	quePayload, err := proto.Marshal(&clientv1.Envelope{
		ReqId: "q",
		Body: &clientv1.Envelope_OpeningDone{
			OpeningDone: &clientv1.OpeningDoneNotify{
				Action:   "que_men",
				Kind:     "missing_suit_done",
				SeatInts: []*clientv1.OpeningSeatInts{{Key: "que_suit", Values: []int32{0, 1, 2, 0}}},
			},
		},
	})
	require.NoError(t, err)
	actionPayload, err := proto.Marshal(&clientv1.Envelope{
		ReqId: "a",
		Body: &clientv1.Envelope_Action{
			Action: &clientv1.ActionNotify{SeatIndex: 0, Action: "discard", Tile: "m1"},
		},
	})
	require.NoError(t, err)

	srv.persistRoomMeta(ctx, "r-meta", 1, &eng.Notification{Kind: eng.KindOpeningDone, Payload: quePayload})
	srv.persistRoomMeta(ctx, "r-meta", 2, &eng.Notification{Kind: eng.KindAction, Payload: actionPayload})

	meta, ok, err := rdb.GetRoomSnapMeta(ctx, "r-meta")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, []int32{0, 1, 2, 0}, meta.QueSuits)
}

func TestApplyNotificationsDoesNotPublishPartialEventsOnPersistFailure(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	t.Cleanup(func() { mock.Close() })

	mock.ExpectBeginTx(pgx.TxOptions{})
	mock.ExpectQuery("SELECT seq FROM room_events").
		WithArgs("r-batch").
		WillReturnRows(pgxmock.NewRows([]string{"seq"}).AddRow(int64(0)))
	mock.ExpectExec("INSERT INTO room_events").
		WithArgs("r-batch", int64(1), string(eng.KindDrawTile), []byte("draw"), int32(-1)).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectExec("INSERT INTO room_events").
		WithArgs("r-batch", int64(2), string(eng.KindAction), []byte("action"), int32(-1)).
		WillReturnError(context.DeadlineExceeded)
	mock.ExpectRollback()

	mr2, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr2.Close)
	rcli2 := goredis.NewClient(&goredis.Options{Addr: mr2.Addr()})
	t.Cleanup(func() { _ = rcli2.Close() })
	rdb2 := redis.NewClientFromUniversal(rcli2)

	ev := postgres.NewRoomEventStore(mock)
	srv := NewGRPCServer(roomsvc.NewServiceWithRule(roomsvc.NewRoomRegistry(), "sichuan_xuezhandaodi_huansanzhang"), ev, nil, nil, rdb2)

	resp, err := srv.applyNotifications(context.Background(), "r-batch", "", []eng.Notification{
		{Kind: eng.KindDrawTile, Payload: []byte("draw"), TargetSeat: eng.BroadcastSeat},
		{Kind: eng.KindAction, Payload: []byte("action"), TargetSeat: eng.BroadcastSeat},
	}, nil)
	require.NoError(t, err)
	require.False(t, resp.GetAccepted())
	require.Contains(t, resp.GetError(), context.DeadlineExceeded.Error())

	// 持久化失败时 Redis 队列应为空，不应推送任何部分事件。
	popCtx, popCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer popCancel()
	data, popErr := rdb2.RoomEventQueuePop(popCtx, "r-batch", 10*time.Millisecond)
	require.NoError(t, popErr)
	require.Nil(t, data, "持久化失败时不应向 Redis 推送任何事件")

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetRoomEventsWithNilStore(t *testing.T) {
	t.Parallel()
	// ev == nil 时应返回空响应而不出错，覆盖 GetRoomEvents 早返回路径。
	srv := NewGRPCServer(nil, nil, nil, nil, nil)
	resp, err := srv.GetRoomEvents(context.Background(), &svcv1.GetRoomEventsRequest{
		RoomId:      "r-nilev",
		SinceCursor: "r-nilev:0",
	})
	require.NoError(t, err)
	require.Empty(t, resp.GetEvents())
}

func TestSetIdempotencyTTL(t *testing.T) {
	t.Parallel()
	// 覆盖 SetIdempotencyTTL 的正常赋值路径。
	srv := NewGRPCServer(nil, nil, nil, nil, nil)
	srv.SetIdempotencyTTL(30 * time.Second)
	require.Equal(t, 30*time.Second, srv.idempotencyTTL)
}
