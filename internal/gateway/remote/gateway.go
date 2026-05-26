package remote

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	svcv1 "racoo.cn/lsp/api/gen/go/v1"
	"racoo.cn/lsp/internal/cluster/discovery"
	"racoo.cn/lsp/internal/cluster/router"
	"racoo.cn/lsp/internal/config"
	"racoo.cn/lsp/internal/handler"
	"racoo.cn/lsp/internal/session"
	"racoo.cn/lsp/internal/store/postgres"
	"racoo.cn/lsp/internal/store/redis"
	"racoo.cn/lsp/pkg/logx"
)

// remoteRoomGateway 通过 gRPC 代理 handler.RoomGateway 接口至集群 lobby/room 服务。
type remoteRoomGateway struct {
	lobby             svcv1.LobbyServiceClient
	defaultRoomAddr   string
	defaultRoomClient svcv1.RoomServiceClient
	hub               *session.Hub
	sess              *session.Manager
	routeCache        *redis.Client
	settlementStore   *postgres.SettlementStore
	router            *router.Etcd
	discovery         *discovery.Etcd

	// pollCtx 是全部 Redis 轮询 goroutine 的根 context，gateway 关闭时统一取消。
	pollCtx context.Context
	pollMu  sync.Mutex
	// pollHandles 记录每个活跃房间的轮询取消函数，Leave 时显式取消对应 goroutine。
	pollHandles map[string]context.CancelFunc

	seatMu                sync.Mutex
	roomSeats             map[string]map[int32]string
	offlineSurrenderAfter time.Duration

	connMu      sync.Mutex
	roomConnMap map[string]*grpc.ClientConn
	roomClients map[string]svcv1.RoomServiceClient
}

// New 根据配置构造远程房间网关，返回网关实例、清理函数和初始化错误。
func New(cfg config.Config, hub *session.Hub, sess *session.Manager, routeCache *redis.Client, settlementStore *postgres.SettlementStore) (handler.RoomGateway, func(), error) {
	lobbyConn, err := grpc.NewClient(cfg.ClusterLobbyAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, fmt.Errorf("dial lobby grpc: %w", err)
	}
	roomConn, err := grpc.NewClient(cfg.ClusterRoomAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		_ = lobbyConn.Close()
		return nil, nil, fmt.Errorf("dial room grpc: %w", err)
	}
	var (
		etcdCli   *clientv3.Client
		roomRoute *router.Etcd
		roomDisc  *discovery.Etcd
	)
	if strings.TrimSpace(cfg.EtcdEndpoints) != "" {
		etcdCli, err = clientv3.New(clientv3.Config{
			Endpoints:   splitCommaSeparated(cfg.EtcdEndpoints),
			DialTimeout: 5 * time.Second,
		})
		if err != nil {
			_ = lobbyConn.Close()
			_ = roomConn.Close()
			return nil, nil, fmt.Errorf("dial etcd: %w", err)
		}
		roomRoute = router.NewEtcd(etcdCli, "/lsp")
		roomDisc = discovery.NewEtcd(etcdCli, "/lsp", 30)
	}
	pollCtx, cancel := context.WithCancel(context.Background())
	gateway := &remoteRoomGateway{
		lobby:                 svcv1.NewLobbyServiceClient(lobbyConn),
		defaultRoomAddr:       cfg.ClusterRoomAddr,
		defaultRoomClient:     svcv1.NewRoomServiceClient(roomConn),
		hub:                   hub,
		sess:                  sess,
		routeCache:            routeCache,
		settlementStore:       settlementStore,
		router:                roomRoute,
		discovery:             roomDisc,
		offlineSurrenderAfter: cfg.Runtime.RoomSurrenderAfterOffline,
		pollCtx:               pollCtx,
		pollHandles:           make(map[string]context.CancelFunc),
		roomSeats:             make(map[string]map[int32]string),
		roomConnMap:           map[string]*grpc.ClientConn{cfg.ClusterRoomAddr: roomConn},
		roomClients:           map[string]svcv1.RoomServiceClient{cfg.ClusterRoomAddr: svcv1.NewRoomServiceClient(roomConn)},
	}
	cleanup := func() {
		cancel()
		_ = lobbyConn.Close()
		gateway.connMu.Lock()
		for addr, conn := range gateway.roomConnMap {
			if conn == nil {
				continue
			}
			_ = conn.Close()
			delete(gateway.roomConnMap, addr)
			delete(gateway.roomClients, addr)
		}
		gateway.connMu.Unlock()
		if etcdCli != nil {
			_ = etcdCli.Close()
		}
	}
	return gateway, cleanup, nil
}

// withOutgoingTrace 将 context 中的 trace_id 写入 gRPC 出站元数据。
func withOutgoingTrace(ctx context.Context) context.Context {
	tid := logx.TraceIDFromContext(ctx)
	if tid == "" {
		return ctx
	}
	return metadata.AppendToOutgoingContext(ctx, "racoo-trace-id", tid)
}

// cloneStringMap 深拷贝字符串映射；nil 或空时返回 nil。
func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
