package main

import (
	"context"
	"net"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	clusterv1 "racoo.cn/lsp/api/gen/go/cluster/v1"
	roomsvc "racoo.cn/lsp/internal/service/room"
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
	for i := 0; i < 64; i++ {
		evt, err := stream.Recv()
		require.NoError(t, err)
		require.Equal(t, "r1", evt.GetRoomId())
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
