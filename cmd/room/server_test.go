package main

import (
	"context"
	"net"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v3"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	clusterv1 "racoo.cn/lsp/api/gen/go/cluster/v1"
	roomsvc "racoo.cn/lsp/internal/service/room"
	"racoo.cn/lsp/internal/store/postgres"
	"racoo.cn/lsp/internal/store/redis"
)

func TestRoomGRPCServerApplyEventAndStream(t *testing.T) {
	t.Parallel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = ln.Close() }()

	grpcSrv := grpc.NewServer()
	srv := newRoomGRPCServer(roomsvc.NewServiceWithRule(roomsvc.NewLobby(), "sichuan_xzdd"), nil, nil, nil, nil)
	registerRoomService(grpcSrv, srv)
	go func() { _ = grpcSrv.Serve(ln) }()
	defer grpcSrv.Stop()

	conn, err := grpc.NewClient(ln.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	client := clusterv1.NewRoomServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	stream, err := client.StreamEvents(ctx, &clusterv1.StreamEventsRequest{RoomId: "r1"})
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		srv.mu.Lock()
		defer srv.mu.Unlock()
		return len(srv.streams["r1"]) == 1
	}, time.Second, 10*time.Millisecond)

	for _, userID := range []string{"u1", "u2", "u3", "u4"} {
		resp, err := client.ApplyEvent(ctx, &clusterv1.ApplyEventRequest{
			RoomId: "r1",
			UserId: userID,
			Body:   &clusterv1.ApplyEventRequest_Ready{Ready: &clusterv1.ReadyEvent{}},
		})
		require.NoError(t, err)
		require.True(t, resp.GetAccepted())
	}

	var gotSettlement bool
	players := []string{"u1", "u2", "u3", "u4"}
	for i := 0; i < 512; i++ {
		evt, err := stream.Recv()
		require.NoError(t, err)
		require.Equal(t, "r1", evt.GetRoomId())
		if draw := evt.GetDrawTile(); draw != nil {
			_, err = client.ApplyEvent(ctx, &clusterv1.ApplyEventRequest{
				RoomId: "r1",
				UserId: players[draw.GetSeatIndex()],
				Body:   &clusterv1.ApplyEventRequest_Discard{Discard: &clusterv1.DiscardEvent{Tile: draw.GetTile()}},
			})
			require.NoError(t, err)
		}
		if action := evt.GetAction(); action != nil {
			switch action.GetAction() {
			case "exchange_three":
				_, err = client.ApplyEvent(ctx, &clusterv1.ApplyEventRequest{
					RoomId: "r1",
					UserId: players[action.GetSeatIndex()],
					Body:   &clusterv1.ApplyEventRequest_ExchangeThree{ExchangeThree: &clusterv1.ExchangeThreeEvent{}},
				})
				require.NoError(t, err)
			case "que_men":
				_, err = client.ApplyEvent(ctx, &clusterv1.ApplyEventRequest{
					RoomId: "r1",
					UserId: players[action.GetSeatIndex()],
					Body:   &clusterv1.ApplyEventRequest_QueMen{QueMen: &clusterv1.QueMenEvent{Suit: 0}},
				})
				require.NoError(t, err)
			case "pong_choice":
				_, err = client.ApplyEvent(ctx, &clusterv1.ApplyEventRequest{
					RoomId: "r1",
					UserId: players[action.GetSeatIndex()],
					Body:   &clusterv1.ApplyEventRequest_Pong{Pong: &clusterv1.PongEvent{}},
				})
				require.NoError(t, err)
			case "gang_choice":
				_, err = client.ApplyEvent(ctx, &clusterv1.ApplyEventRequest{
					RoomId: "r1",
					UserId: players[action.GetSeatIndex()],
					Body:   &clusterv1.ApplyEventRequest_Gang{Gang: &clusterv1.GangEvent{Tile: action.GetTile()}},
				})
				require.NoError(t, err)
			case "hu_choice", "qiang_gang_choice", "tsumo_choice":
				_, err = client.ApplyEvent(ctx, &clusterv1.ApplyEventRequest{
					RoomId: "r1",
					UserId: players[action.GetSeatIndex()],
					Body:   &clusterv1.ApplyEventRequest_Hu{Hu: &clusterv1.HuEvent{}},
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

	s := newRoomGRPCServer(roomsvc.NewServiceWithRule(roomsvc.NewLobby(), "sichuan_xzdd"), nil, nil, nil, rdb)
	ctx := context.Background()

	s.setReady(false)
	resp1, err := s.ApplyEvent(ctx, &clusterv1.ApplyEventRequest{
		RoomId:         "r-idem",
		UserId:         "u1",
		IdempotencyKey: "k-retry",
		Body:           &clusterv1.ApplyEventRequest_Ready{Ready: &clusterv1.ReadyEvent{}},
	})
	require.NoError(t, err)
	require.False(t, resp1.GetAccepted())

	s.setReady(true)
	resp2, err := s.ApplyEvent(ctx, &clusterv1.ApplyEventRequest{
		RoomId:         "r-idem",
		UserId:         "u1",
		IdempotencyKey: "k-retry",
		Body:           &clusterv1.ApplyEventRequest_Ready{Ready: &clusterv1.ReadyEvent{}},
	})
	require.NoError(t, err)
	require.True(t, resp2.GetAccepted())

	resp3, err := s.ApplyEvent(ctx, &clusterv1.ApplyEventRequest{
		RoomId:         "r-idem",
		UserId:         "u1",
		IdempotencyKey: "k-retry",
		Body:           &clusterv1.ApplyEventRequest_Ready{Ready: &clusterv1.ReadyEvent{}},
	})
	require.NoError(t, err)
	require.True(t, resp3.GetAccepted())
}

func TestSnapshotRoomIncludesRoundView(t *testing.T) {
	t.Parallel()

	srv := newRoomGRPCServer(roomsvc.NewServiceWithRule(roomsvc.NewLobby(), "sichuan_xzdd"), nil, nil, nil, nil)
	ctx := context.Background()
	for _, userID := range []string{"u1", "u2", "u3", "u4"} {
		_, err := srv.ApplyEvent(ctx, &clusterv1.ApplyEventRequest{
			RoomId: "r-snap",
			UserId: userID,
			Body:   &clusterv1.ApplyEventRequest_Ready{Ready: &clusterv1.ReadyEvent{}},
		})
		require.NoError(t, err)
	}

	snap, err := srv.SnapshotRoom(ctx, &clusterv1.SnapshotRoomRequest{RoomId: "r-snap"})
	require.NoError(t, err)
	require.Equal(t, "playing", snap.GetState())
	require.EqualValues(t, 0, snap.GetActingSeat())
	require.Equal(t, "exchange_three", snap.GetWaitingAction())
	require.Contains(t, snap.GetAvailableActions(), "exchange_three")
}

func TestClusterToClientConverters(t *testing.T) {
	t.Parallel()

	scores := clusterSeatScoresToClient([]*clusterv1.SeatScore{{SeatIndex: 1, UserId: "u1", TotalFan: 8, Skipped: true}})
	require.Equal(t, "u1", scores[0].GetUserId())
	require.EqualValues(t, 8, scores[0].GetTotalFan())
	require.True(t, scores[0].GetSkipped())

	penalties := clusterPenaltiesToClient([]*clusterv1.PenaltyItem{{Reason: "查大叫", FromSeat: 0, ToSeat: 2, Amount: 16}})
	require.Equal(t, "查大叫", penalties[0].GetReason())
	require.EqualValues(t, 16, penalties[0].GetAmount())
}

func TestMapPGRowToEvent(t *testing.T) {
	t.Parallel()

	payload, err := proto.Marshal(&clientv1.Envelope{
		Body: &clientv1.Envelope_StartGame{StartGame: &clientv1.StartGameNotify{DealerSeat: 2}},
	})
	require.NoError(t, err)

	evt, err := mapPGRowToEvent("r-pg", postgres.RoomEventRow{Seq: 7, Kind: string(roomsvc.KindStartGame), Payload: payload})
	require.NoError(t, err)
	require.Equal(t, "r-pg:7", evt.GetCursor())
	require.EqualValues(t, 2, evt.GetStartGame().GetDealerSeat())
}

func TestPersistRoomMetaKeepsQueSuits(t *testing.T) {
	t.Parallel()

	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)
	rcli := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rcli.Close() })
	rdb := redis.NewClientFromUniversal(rcli)

	rooms := roomsvc.NewServiceWithRule(roomsvc.NewLobby(), "sichuan_xzdd")
	for _, uid := range []string{"u1", "u2", "u3", "u4"} {
		_, err := rooms.Join(context.Background(), "r-meta", uid)
		require.NoError(t, err)
	}
	srv := newRoomGRPCServer(rooms, nil, nil, nil, rdb)
	ctx := context.Background()

	quePayload, err := proto.Marshal(&clientv1.Envelope{
		ReqId: "q",
		Body: &clientv1.Envelope_QueMenDone{
			QueMenDone: &clientv1.QueMenDoneNotify{QueSuitBySeat: []int32{0, 1, 2, 0}},
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

	srv.persistRoomMeta(ctx, "r-meta", 1, &roomsvc.Notification{Kind: roomsvc.KindQueMenDone, Payload: quePayload})
	srv.persistRoomMeta(ctx, "r-meta", 2, &roomsvc.Notification{Kind: roomsvc.KindAction, Payload: actionPayload})

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
		WithArgs("r-batch", int64(1), string(roomsvc.KindDrawTile), []byte("draw"), int32(-1)).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectExec("INSERT INTO room_events").
		WithArgs("r-batch", int64(2), string(roomsvc.KindAction), []byte("action"), int32(-1)).
		WillReturnError(context.DeadlineExceeded)
	mock.ExpectRollback()

	ev := postgres.NewRoomEventStore(mock)
	srv := newRoomGRPCServer(roomsvc.NewServiceWithRule(roomsvc.NewLobby(), "sichuan_xzdd"), ev, nil, nil, nil)
	ch := make(chan *clusterv1.RoomServiceStreamEventsResponse, 1)
	srv.streams["r-batch"] = []chan *clusterv1.RoomServiceStreamEventsResponse{ch}

	resp, err := srv.applyNotifications(context.Background(), "r-batch", "", []roomsvc.Notification{
		{Kind: roomsvc.KindDrawTile, Payload: []byte("draw"), TargetSeat: roomsvc.BroadcastSeat},
		{Kind: roomsvc.KindAction, Payload: []byte("action"), TargetSeat: roomsvc.BroadcastSeat},
	}, nil)
	require.NoError(t, err)
	require.False(t, resp.GetAccepted())
	require.Contains(t, resp.GetError(), context.DeadlineExceeded.Error())
	select {
	case evt := <-ch:
		t.Fatalf("unexpected published event: %+v", evt)
	default:
	}
	require.NoError(t, mock.ExpectationsWereMet())
}
