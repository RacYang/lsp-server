package app

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	clusterv1 "racoo.cn/lsp/api/gen/go/cluster/v1"
	"racoo.cn/lsp/internal/config"
	"racoo.cn/lsp/internal/handler"
	"racoo.cn/lsp/internal/net/msgid"
	"racoo.cn/lsp/internal/session"
	"racoo.cn/lsp/pkg/logx"
)

type remoteRoomGateway struct {
	lobby clusterv1.LobbyServiceClient
	room  clusterv1.RoomServiceClient
	hub   *session.Hub

	streamCtx context.Context
	mu        sync.Mutex
	streaming map[string]struct{}
}

func newRemoteRoomGateway(cfg config.Config, hub *session.Hub) (handler.RoomGateway, func(), error) {
	lobbyConn, err := grpc.NewClient(cfg.ClusterLobbyAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, fmt.Errorf("dial lobby grpc: %w", err)
	}
	roomConn, err := grpc.NewClient(cfg.ClusterRoomAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		_ = lobbyConn.Close()
		return nil, nil, fmt.Errorf("dial room grpc: %w", err)
	}
	streamCtx, cancel := context.WithCancel(context.Background())
	gateway := &remoteRoomGateway{
		lobby:     clusterv1.NewLobbyServiceClient(lobbyConn),
		room:      clusterv1.NewRoomServiceClient(roomConn),
		hub:       hub,
		streamCtx: streamCtx,
		streaming: make(map[string]struct{}),
	}
	cleanup := func() {
		cancel()
		_ = lobbyConn.Close()
		_ = roomConn.Close()
	}
	return gateway, cleanup, nil
}

// Join 通过 LobbyService 分配座位，并在首次进房时建立 room 事件订阅。
func (g *remoteRoomGateway) Join(ctx context.Context, roomID, userID string) (int, error) {
	if g == nil {
		return -1, fmt.Errorf("nil remote room gateway")
	}
	resp, err := g.lobby.JoinRoom(ctx, &clusterv1.JoinRoomRequest{
		RoomId: roomID,
		UserId: userID,
	})
	if err != nil {
		return -1, err
	}
	if resp.GetError() != "" {
		return -1, errors.New(resp.GetError())
	}
	if err := g.ensureRoomStream(roomID); err != nil {
		return -1, err
	}
	return int(resp.GetSeatIndex()), nil
}

// Ready 将准备命令发给 RoomService；实际推送由后台事件流转发到客户端。
func (g *remoteRoomGateway) Ready(ctx context.Context, roomID, userID string) (func(), error) {
	if g == nil {
		return nil, fmt.Errorf("nil remote room gateway")
	}
	resp, err := g.room.ApplyEvent(ctx, &clusterv1.ApplyEventRequest{
		RoomId: roomID,
		UserId: userID,
		Body:   &clusterv1.ApplyEventRequest_Ready{Ready: &clusterv1.ReadyEvent{}},
	})
	if err != nil {
		return nil, err
	}
	if resp.GetError() != "" {
		return nil, errors.New(resp.GetError())
	}
	if !resp.GetAccepted() {
		return nil, fmt.Errorf("room apply event rejected")
	}
	return nil, nil
}

func (g *remoteRoomGateway) ensureRoomStream(roomID string) error {
	g.mu.Lock()
	if _, ok := g.streaming[roomID]; ok {
		g.mu.Unlock()
		return nil
	}
	g.streaming[roomID] = struct{}{}
	g.mu.Unlock()

	stream, err := g.room.StreamEvents(g.streamCtx, &clusterv1.StreamEventsRequest{RoomId: roomID})
	if err != nil {
		g.mu.Lock()
		delete(g.streaming, roomID)
		g.mu.Unlock()
		return fmt.Errorf("subscribe room stream: %w", err)
	}
	go g.consumeRoomStream(roomID, stream)
	return nil
}

func (g *remoteRoomGateway) consumeRoomStream(roomID string, stream grpc.ServerStreamingClient[clusterv1.RoomServiceStreamEventsResponse]) {
	defer func() {
		g.mu.Lock()
		delete(g.streaming, roomID)
		g.mu.Unlock()
	}()
	for {
		evt, err := stream.Recv()
		if err != nil {
			return
		}
		msgID, payload, err := encodeClusterRoomEvent(evt)
		if err != nil {
			logx.Warn(context.Background(), "房间事件转客户端推送失败", "trace_id", "", "user_id", "", "room_id", roomID, "err", err.Error())
			continue
		}
		if g.hub != nil {
			g.hub.Broadcast(roomID, msgID, payload)
		}
	}
}

func encodeClusterRoomEvent(evt *clusterv1.RoomServiceStreamEventsResponse) (uint16, []byte, error) {
	if evt == nil {
		return 0, nil, fmt.Errorf("nil room event")
	}
	switch body := evt.Body.(type) {
	case *clusterv1.RoomServiceStreamEventsResponse_StartGame:
		return marshalClientEnvelope(msgid.StartGame, &clientv1.Envelope{
			ReqId: evt.GetCursor(),
			Body: &clientv1.Envelope_StartGame{StartGame: &clientv1.StartGameNotify{
				RoomId:     evt.GetRoomId(),
				DealerSeat: body.StartGame.GetDealerSeat(),
			}},
		})
	case *clusterv1.RoomServiceStreamEventsResponse_DrawTile:
		return marshalClientEnvelope(msgid.DrawTile, &clientv1.Envelope{
			ReqId: evt.GetCursor(),
			Body: &clientv1.Envelope_DrawTile{DrawTile: &clientv1.DrawTileNotify{
				SeatIndex: body.DrawTile.GetSeatIndex(),
				Tile:      body.DrawTile.GetTile(),
			}},
		})
	case *clusterv1.RoomServiceStreamEventsResponse_Action:
		return marshalClientEnvelope(msgid.ActionNotify, &clientv1.Envelope{
			ReqId: evt.GetCursor(),
			Body: &clientv1.Envelope_Action{Action: &clientv1.ActionNotify{
				SeatIndex: body.Action.GetSeatIndex(),
				Action:    body.Action.GetAction(),
				Tile:      body.Action.GetTile(),
			}},
		})
	case *clusterv1.RoomServiceStreamEventsResponse_Settlement:
		return marshalClientEnvelope(msgid.Settlement, &clientv1.Envelope{
			ReqId: evt.GetCursor(),
			Body: &clientv1.Envelope_Settlement{Settlement: &clientv1.SettlementNotify{
				RoomId:        evt.GetRoomId(),
				WinnerUserIds: append([]string(nil), body.Settlement.GetWinnerUserIds()...),
				TotalFan:      body.Settlement.GetTotalFan(),
				DetailText:    body.Settlement.GetDetailText(),
			}},
		})
	case *clusterv1.RoomServiceStreamEventsResponse_ExchangeThreeDone:
		perSeat := make([]*clientv1.SeatTiles, 0, len(body.ExchangeThreeDone.GetSeatTiles()))
		for _, item := range body.ExchangeThreeDone.GetSeatTiles() {
			perSeat = append(perSeat, &clientv1.SeatTiles{
				SeatIndex: item.GetSeatIndex(),
				Tiles:     append([]string(nil), item.GetTiles()...),
			})
		}
		return marshalClientEnvelope(msgid.ExchangeThreeDone, &clientv1.Envelope{
			ReqId: evt.GetCursor(),
			Body: &clientv1.Envelope_ExchangeThreeDone{ExchangeThreeDone: &clientv1.ExchangeThreeDoneNotify{
				PerSeat: perSeat,
			}},
		})
	case *clusterv1.RoomServiceStreamEventsResponse_QueMenDone:
		return marshalClientEnvelope(msgid.QueMenDone, &clientv1.Envelope{
			ReqId: evt.GetCursor(),
			Body: &clientv1.Envelope_QueMenDone{QueMenDone: &clientv1.QueMenDoneNotify{
				QueSuitBySeat: append([]int32(nil), body.QueMenDone.GetQueSuitBySeat()...),
			}},
		})
	case *clusterv1.RoomServiceStreamEventsResponse_RouteRedirect:
		return marshalClientEnvelope(msgid.RouteRedirectNotify, &clientv1.Envelope{
			ReqId: evt.GetCursor(),
			Body: &clientv1.Envelope_RouteRedirect{RouteRedirect: &clientv1.RouteRedirectNotify{
				WsUrl:  body.RouteRedirect.GetWsUrl(),
				Reason: body.RouteRedirect.GetReason(),
			}},
		})
	default:
		return 0, nil, fmt.Errorf("unknown room event body")
	}
}

func marshalClientEnvelope(msgID uint16, env *clientv1.Envelope) (uint16, []byte, error) {
	payload, err := proto.Marshal(env)
	if err != nil {
		return 0, nil, err
	}
	return msgID, payload, nil
}
