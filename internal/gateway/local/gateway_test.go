package local

import (
	"context"
	"testing"

	miniredis "github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	roomsvc "racoo.cn/lsp/internal/service/room"
	"racoo.cn/lsp/internal/session"
	"racoo.cn/lsp/internal/store/redis"
)

func TestLocalRoomGatewayListRules(t *testing.T) {
	t.Parallel()
	g := NewLocalRoomGateway(nil, nil, nil)
	rules, err := g.ListRules(context.Background())
	require.NoError(t, err)
	require.Len(t, rules, 3)
	require.Equal(t, "guobiao_jingji_biaozhun", rules[0].GetRuleId())
	require.Equal(t, "sichuan_xuezhandaodi_biaozhun", rules[1].GetRuleId())
	require.Equal(t, "sichuan_xuezhandaodi_huansanzhang", rules[2].GetRuleId())
	require.Contains(t, rules[0].GetEnabledFeatures(), "mcr_81_fans")
	require.Contains(t, rules[0].GetEnabledFeatures(), "full_tiles")
	require.NotContains(t, rules[1].GetEnabledFeatures(), "exchange_three")
	require.Contains(t, rules[2].GetEnabledFeatures(), "exchange_three")
}

func TestLocalRoomGatewayNilErrors(t *testing.T) {
	t.Parallel()
	var g *LocalRoomGateway
	ctx := context.Background()
	_, err := g.Join(ctx, "r", "u")
	require.Error(t, err)
	_, err = g.Ready(ctx, "r", "u")
	require.Error(t, err)
	_, err = g.Leave(ctx, "r", "u")
	require.Error(t, err)
	_, err = g.Discard(ctx, "r", "u", "1m", nil)
	require.Error(t, err)
	_, err = g.Pong(ctx, "r", "u", nil)
	require.Error(t, err)
	_, err = g.Gang(ctx, "r", "u", "1m", nil)
	require.Error(t, err)
	_, err = g.Hu(ctx, "r", "u", nil)
	require.Error(t, err)
	_, err = g.OpeningAction(ctx, "r", "u", "exchange_three", nil, 0, 0, nil, nil)
	require.Error(t, err)
	_, err = g.OpeningAction(ctx, "r", "u", "que_men", nil, 0, 0, nil, nil)
	require.Error(t, err)
	_, err = g.Resume(ctx, "tok")
	require.Error(t, err)
}

func TestLocalRoomGatewayJoinSmoke(t *testing.T) {
	t.Parallel()
	svc := roomsvc.NewService(roomsvc.NewRoomRegistry())
	g := NewLocalRoomGateway(svc, session.NewHub(), nil)
	seat, err := g.Join(context.Background(), "local-room", "u-local")
	require.NoError(t, err)
	require.GreaterOrEqual(t, seat, 0)
}

func TestLocalRoomGatewayResumeRequiresSession(t *testing.T) {
	t.Parallel()
	svc := roomsvc.NewService(roomsvc.NewRoomRegistry())
	g := NewLocalRoomGateway(svc, session.NewHub(), nil)
	_, err := g.Resume(context.Background(), "any-token")
	require.Error(t, err)
}

func TestLocalRoomGatewayEnsureSubscriptionNoOp(t *testing.T) {
	t.Parallel()
	svc := roomsvc.NewService(roomsvc.NewRoomRegistry())
	g := NewLocalRoomGateway(svc, session.NewHub(), nil)
	require.NoError(t, g.EnsureRoomEventSubscription(context.Background(), "r", "c"))
}

func TestLocalRoomGatewaySendsPerSeatNotifications(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := roomsvc.NewService(roomsvc.NewRoomRegistry())
	for _, uid := range []string{"u0", "u1", "u2", "u3"} {
		_, err := svc.Join(ctx, "project-room", uid)
		require.NoError(t, err)
	}
	g := NewLocalRoomGateway(svc, session.NewHub(), nil)
	// 引擎已在生成时展开为每座位一条独立通知；此处模拟四条逐一调用
	for seat := roomsvc.Seat(0); seat < 4; seat++ {
		payload, err := proto.Marshal(&clientv1.Envelope{
			Body: &clientv1.Envelope_DrawTile{DrawTile: &clientv1.DrawTileNotify{SeatIndex: int32(seat), Tile: ""}},
		})
		require.NoError(t, err)
		g.sendNotification("project-room", roomsvc.Notification{
			Kind:       roomsvc.KindDrawTile,
			Payload:    payload,
			TargetSeat: seat,
		})
	}
}

func TestLocalRoomGatewayResumeWithRedisSession(t *testing.T) {
	t.Parallel()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)

	rcli := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rcli.Close() })
	cli := redis.NewClientFromUniversal(rcli)
	mgr := session.NewManager(cli)

	lobby := roomsvc.NewRoomRegistry()
	svc := roomsvc.NewService(lobby)
	hub := session.NewHub()
	gw := NewLocalRoomGateway(svc, hub, mgr)

	ctx := context.Background()
	tok, err := mgr.Issue(ctx, "resume-user")
	require.NoError(t, err)

	_, err = gw.Join(ctx, "resume-room", "resume-user")
	require.NoError(t, err)
	for _, uid := range []string{"u2", "u3", "u4"} {
		_, err = gw.Join(ctx, "resume-room", uid)
		require.NoError(t, err)
	}
	for _, uid := range []string{"resume-user", "u2", "u3", "u4"} {
		_, err = gw.Ready(ctx, "resume-room", uid)
		require.NoError(t, err)
	}
	for _, uid := range []string{"resume-user", "u2", "u3", "u4"} {
		view, ok, viewErr := svc.RoundView(ctx, "resume-room")
		require.NoError(t, viewErr)
		require.True(t, ok)
		seat := seatIndexForUser(view.PlayerIDs[:], uid)
		require.GreaterOrEqual(t, seat, 0)
		_, err = gw.OpeningAction(ctx, "resume-room", uid, "exchange_three", view.HandsBySeat[seat][:3], 0, 0, nil, nil)
		require.NoError(t, err)
	}
	for _, uid := range []string{"resume-user", "u2", "u3", "u4"} {
		_, err = gw.OpeningAction(ctx, "resume-room", uid, "que_men", nil, 0, 0, nil, nil)
		require.NoError(t, err)
	}
	require.NoError(t, mgr.BindRoom(ctx, "resume-user", "resume-room"))

	res, err := gw.Resume(ctx, tok)
	require.NoError(t, err)
	require.Equal(t, "resume-user", res.UserID)
	require.Equal(t, "resume-room", res.RoomID)
	require.NotNil(t, res.Snapshot)
	require.Equal(t, "resume-room", res.Snapshot.GetRoomId())
	require.Len(t, res.Snapshot.GetQueSuitBySeat(), 4)
	require.NotEmpty(t, res.Snapshot.GetYourHandTiles())
	require.Len(t, res.Snapshot.GetDiscardsBySeat(), 4)
	require.Len(t, res.Snapshot.GetMeldsBySeat(), 4)
}

func TestLocalRoomGatewayReadyBroadcastSkippedWhenHubNil(t *testing.T) {
	t.Parallel()
	svc := roomsvc.NewService(roomsvc.NewRoomRegistry())
	gw := NewLocalRoomGateway(svc, nil, nil)
	ctx := context.Background()

	_, err := gw.Join(ctx, "ready-room", "p0")
	require.NoError(t, err)
	_, err = gw.Join(ctx, "ready-room", "p1")
	require.NoError(t, err)
	_, err = gw.Join(ctx, "ready-room", "p2")
	require.NoError(t, err)
	_, err = gw.Join(ctx, "ready-room", "p3")
	require.NoError(t, err)

	cb, err := gw.Ready(ctx, "ready-room", "p0")
	require.NoError(t, err)
	require.NotNil(t, cb)
	cb()
}

func TestLocalRoomGatewayAddBotReturnsAuthoritativeSeatInfo(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := roomsvc.NewService(roomsvc.NewRoomRegistry())
	gw := NewLocalRoomGateway(svc, nil, nil)

	_, err := gw.Join(ctx, "bot-room", "human")
	require.NoError(t, err)

	added, after, err := gw.AddBot(ctx, "bot-room", "human", 1, "", "")
	require.NoError(t, err)
	require.Len(t, added, 1)
	require.NotNil(t, after)
	require.Equal(t, "ready", added[0].GetStatus())
	require.True(t, added[0].GetOnline())
	require.True(t, added[0].GetAutoPlay())
	require.True(t, added[0].GetIsBot())
}

func TestLocalRoomGatewayResumeRoomMissing(t *testing.T) {
	t.Parallel()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)
	rcli := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rcli.Close() })
	cli := redis.NewClientFromUniversal(rcli)
	mgr := session.NewManager(cli)

	svc := roomsvc.NewService(roomsvc.NewRoomRegistry())
	gw := NewLocalRoomGateway(svc, session.NewHub(), mgr)
	ctx := context.Background()

	tok, err := mgr.Issue(ctx, "orphan")
	require.NoError(t, err)
	require.NoError(t, mgr.BindRoom(ctx, "orphan", "never-created-room"))

	_, err = gw.Resume(ctx, tok)
	require.Error(t, err)
}

// TestPlayerJourney_L3_1_RoomAcceptsAutoMatchLocal 锁定 spec [L3.1] 的本地侧契约：
// 仅 waiting/ready 阶段（或房间尚未在房服层登记）才允许 AutoMatch 加入；其余阶段必须跳过。
// 这条契约必须与 internal/app/gate_remote.go 的 remoteRoomGateway.roomAcceptsAutoMatch 同语义，
// 否则违反 spec [G3]（local 与 cluster 模式事实集等价）。
func TestPlayerJourney_L3_1_RoomAcceptsAutoMatchLocal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		state    string
		found    bool
		expected bool
	}{
		{name: "not_registered_yet_treated_as_joinable", state: "", found: false, expected: true},
		{name: "empty_state_joinable", state: "", found: true, expected: true},
		{name: "waiting_joinable", state: "waiting", found: true, expected: true},
		{name: "ready_joinable", state: "ready", found: true, expected: true},
		{name: "playing_skipped", state: "playing", found: true, expected: false},
		{name: "settling_skipped", state: "settling", found: true, expected: false},
		{name: "closed_skipped", state: "closed", found: true, expected: false},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			probe := &stubRoomStateProbe{state: tt.state, found: tt.found}
			require.Equal(t, tt.expected, roomAcceptsAutoMatchLocal(probe, "R1"),
				"[L3.1] roomAcceptsAutoMatchLocal(state=%q, found=%v)", tt.state, tt.found)
		})
	}

	t.Run("nil_probe_rejects", func(t *testing.T) {
		t.Parallel()
		require.False(t, roomAcceptsAutoMatchLocal(nil, "R1"),
			"[L3.1] nil probe 必须按拒绝处理，避免无来源信任")
	})
}

// TestPlayerJourney_L3_2_AutoMatchFallsBackToCreateRoomLocal 锁定 spec [L3.2]：
// AutoMatch 找不到可加入现房时必须新建公开房并占座 0；玩家无需额外确认。
// 这里大厅与房服都空，AutoMatch 应当回落到 CreateRoom 路径并返回 seat=0。
func TestPlayerJourney_L3_2_AutoMatchFallsBackToCreateRoomLocal(t *testing.T) {
	t.Parallel()
	svc := roomsvc.NewService(roomsvc.NewRoomRegistry())
	gw := NewLocalRoomGateway(svc, session.NewHub(), nil)

	roomID, seat, err := gw.AutoMatch(context.Background(), "sichuan_xuezhandaodi_huansanzhang", "u-quickstart", false)
	require.NoError(t, err, "[L3.2] 空大厅时 AutoMatch 必须落到 CreateRoom 成功")
	require.NotEmpty(t, roomID, "[L3.2] CreateRoom 必须返回 room_id")
	require.Equal(t, 0, seat, "[L3.2] AutoMatch 创建公开房后必须占座 0")
}

type stubRoomStateProbe struct {
	state string
	found bool
}

func (s *stubRoomStateProbe) RoomSnapshot(_ string) ([]string, string, [4]bool, bool) {
	return nil, s.state, [4]bool{}, s.found
}

func TestLocalRoomGatewayResumeLobbySession(t *testing.T) {
	t.Parallel()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)
	rcli := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rcli.Close() })
	cli := redis.NewClientFromUniversal(rcli)
	mgr := session.NewManager(cli)

	gw := NewLocalRoomGateway(roomsvc.NewService(roomsvc.NewRoomRegistry()), session.NewHub(), mgr)
	ctx := context.Background()

	tok, err := mgr.Issue(ctx, "lobby-user")
	require.NoError(t, err)
	res, err := gw.Resume(ctx, tok)
	require.NoError(t, err)
	require.Equal(t, "lobby-user", res.UserID)
	require.Empty(t, res.RoomID)
	require.False(t, res.Resumed)
	require.Nil(t, res.Snapshot)
}
