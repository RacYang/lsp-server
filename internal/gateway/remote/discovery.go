package remote

import (
	"context"
	"fmt"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	svcv1 "racoo.cn/lsp/api/gen/go/v1"

	"racoo.cn/lsp/internal/cluster"
	"racoo.cn/lsp/internal/protocol"
	"racoo.cn/lsp/internal/store/redis"
	"racoo.cn/lsp/pkg/logx"
)

// roomClientForRoom 按房间 ID 查找所属节点地址，并返回对应的 gRPC 客户端与地址。
func (g *remoteRoomGateway) roomClientForRoom(ctx context.Context, roomID string) (svcv1.RoomServiceClient, string, error) {
	if g == nil {
		return nil, "", fmt.Errorf("nil remote room gateway")
	}
	addr, err := g.roomAddressForRoom(ctx, roomID)
	if err != nil {
		return nil, "", err
	}
	client, err := g.roomClientForAddr(addr)
	if err != nil {
		return nil, "", err
	}
	return client, addr, nil
}

// roomAddressForRoom 优先通过 etcd 路由解析房间节点地址；未配置 etcd 时回退到默认地址。
func (g *remoteRoomGateway) roomAddressForRoom(ctx context.Context, roomID string) (string, error) {
	if g == nil {
		return "", fmt.Errorf("nil remote room gateway")
	}
	if g.router == nil || g.discovery == nil {
		if g.defaultRoomAddr == "" {
			return "", fmt.Errorf("room address unavailable")
		}
		return g.defaultRoomAddr, nil
	}
	cachedNodeID := ""
	if g.routeCache != nil {
		if rec, ok, err := g.routeCache.GetRoomRouteCache(ctx, roomID); err == nil && ok {
			cachedNodeID = rec.RoomNodeID
		}
	}
	resolvedNodeID, ok, err := g.router.ResolveRoomOwner(ctx, roomID)
	if err != nil {
		return "", err
	}
	if !ok {
		if g.routeCache != nil {
			if err := g.routeCache.DeleteRoomRouteCache(ctx, roomID); err != nil {
				logx.Warn(logx.WithRoomID(ctx, roomID), "删除房间路由缓存失败", "err", err.Error())
			}
		}
		return "", fmt.Errorf("room owner not found: %s", roomID)
	}
	nodeID := resolvedNodeID
	if g.routeCache != nil && cachedNodeID != resolvedNodeID {
		if err := g.routeCache.PutRoomRouteCache(ctx, roomID, redis.RouteRecord{RoomNodeID: resolvedNodeID}, 0); err != nil {
			logx.Warn(logx.WithRoomID(ctx, roomID), "写入房间路由缓存失败", "err", err.Error())
		}
	}
	nodeInfo, ok, err := g.discovery.ResolveNode(ctx, cluster.KindRoom, nodeID)
	if err != nil {
		return "", err
	}
	if !ok || strings.TrimSpace(nodeInfo.Meta.AdvertiseAddr) == "" {
		if g.defaultRoomAddr != "" && nodeID == "room-local" {
			return g.defaultRoomAddr, nil
		}
		return "", fmt.Errorf("room node not ready: %s", nodeID)
	}
	return nodeInfo.Meta.AdvertiseAddr, nil
}

// roomClientForAddr 从连接池返回或新建指定地址的 gRPC 客户端。
// 若已有连接处于 TransientFailure 状态，则从缓存移除并重建，避免用失效连接发请求。
func (g *remoteRoomGateway) roomClientForAddr(addr string) (svcv1.RoomServiceClient, error) {
	g.connMu.Lock()
	defer g.connMu.Unlock()
	// 先检查物理连接健康度：TransientFailure 时清除并重建。
	if conn := g.roomConnMap[addr]; conn != nil {
		if conn.GetState() == connectivity.TransientFailure {
			_ = conn.Close()
			delete(g.roomConnMap, addr)
			delete(g.roomClients, addr)
		}
	}
	// 已有健康客户端（含测试中预注入的 fake 客户端）直接返回。
	if client := g.roomClients[addr]; client != nil {
		return client, nil
	}
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpcKeepaliveOpt)
	if err != nil {
		return nil, fmt.Errorf("dial room grpc %s: %w", addr, err)
	}
	client := svcv1.NewRoomServiceClient(conn)
	if g.roomConnMap == nil {
		g.roomConnMap = make(map[string]*grpc.ClientConn)
	}
	if g.roomClients == nil {
		g.roomClients = make(map[string]svcv1.RoomServiceClient)
	}
	g.roomConnMap[addr] = conn
	g.roomClients[addr] = client
	return client, nil
}

// splitCommaSeparated 将逗号分隔字符串拆分为非空部分并去除空白。
func splitCommaSeparated(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// retryGRPC 对 Unavailable 与 DeadlineExceeded 错误最多重试 8 次，每次间隔 100ms。
func retryGRPC(ctx context.Context, fn func(context.Context) error) error {
	var lastErr error
	for attempt := 0; attempt < 8; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		callCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		err := fn(callCtx)
		cancel()
		if err == nil {
			return nil
		}
		lastErr = err
		st, ok := status.FromError(err)
		if !ok || (st.Code() != codes.Unavailable && st.Code() != codes.DeadlineExceeded) {
			return err
		}
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
			timer.Stop()
		}
	}
	return lastErr
}

// encodeClusterRoomEvent 将 RoomServiceStreamEventsResponse 编码为帧；
// 由于 body 字段已直接使用 client.v1 类型，无须字段级转译，只需封装 Envelope。
func encodeClusterRoomEvent(evt *svcv1.RoomServiceStreamEventsResponse) (uint16, []byte, error) {
	if evt == nil {
		return 0, nil, fmt.Errorf("nil room event")
	}
	switch body := evt.Body.(type) {
	case *svcv1.RoomServiceStreamEventsResponse_InitialDeal:
		return marshalClientEnvelope(protocol.InitialDealNotify, &clientv1.Envelope{
			ReqId: evt.GetCursor(),
			Body:  &clientv1.Envelope_InitialDeal{InitialDeal: body.InitialDeal},
		})
	case *svcv1.RoomServiceStreamEventsResponse_StartGame:
		return marshalClientEnvelope(protocol.StartGame, &clientv1.Envelope{
			ReqId: evt.GetCursor(),
			Body:  &clientv1.Envelope_StartGame{StartGame: body.StartGame},
		})
	case *svcv1.RoomServiceStreamEventsResponse_DrawTile:
		return marshalClientEnvelope(protocol.DrawTile, &clientv1.Envelope{
			ReqId: evt.GetCursor(),
			Body:  &clientv1.Envelope_DrawTile{DrawTile: body.DrawTile},
		})
	case *svcv1.RoomServiceStreamEventsResponse_Action:
		return marshalClientEnvelope(protocol.ActionNotify, &clientv1.Envelope{
			ReqId: evt.GetCursor(),
			Body:  &clientv1.Envelope_Action{Action: body.Action},
		})
	case *svcv1.RoomServiceStreamEventsResponse_Settlement:
		return marshalClientEnvelope(protocol.Settlement, &clientv1.Envelope{
			ReqId: evt.GetCursor(),
			Body:  &clientv1.Envelope_Settlement{Settlement: body.Settlement},
		})
	case *svcv1.RoomServiceStreamEventsResponse_OpeningDone:
		return marshalClientEnvelope(protocol.OpeningDone, &clientv1.Envelope{
			ReqId: evt.GetCursor(),
			Body:  &clientv1.Envelope_OpeningDone{OpeningDone: body.OpeningDone},
		})
	case *svcv1.RoomServiceStreamEventsResponse_RouteRedirect:
		return marshalClientEnvelope(protocol.RouteRedirectNotify, &clientv1.Envelope{
			ReqId: evt.GetCursor(),
			Body:  &clientv1.Envelope_RouteRedirect{RouteRedirect: body.RouteRedirect},
		})
	default:
		return 0, nil, fmt.Errorf("unknown room event body")
	}
}

// marshalClientEnvelope 序列化客户端信封为 proto 字节并附带消息 ID。
func marshalClientEnvelope(msgID uint16, env *clientv1.Envelope) (uint16, []byte, error) {
	payload, err := proto.Marshal(env)
	if err != nil {
		return 0, nil, err
	}
	return msgID, payload, nil
}
