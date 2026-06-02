package handler

import (
	"testing"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"

	localgateway "racoo.cn/lsp/internal/gateway/local"
	"racoo.cn/lsp/internal/protocol"
	roomsvc "racoo.cn/lsp/internal/service/room"
	"racoo.cn/lsp/internal/session"
)

func TestWebSocketLobbyCreateListAndAutoMatch(t *testing.T) {
	t.Parallel()
	roomRegistry := roomsvc.NewRoomRegistry()
	roomService := roomsvc.NewService(roomRegistry)
	hub := session.NewHub()
	srv := wsTestServer(t, Deps{Rooms: localgateway.NewLocalRoomGateway(roomService, hub, nil), Hub: hub})

	creator := dialWS(t, srv)
	loginOnly(t, creator, "creator")
	create := &clientv1.Envelope{ReqId: "create", IdempotencyKey: "idem-create", Body: &clientv1.Envelope_CreateRoomReq{
		CreateRoomReq: &clientv1.CreateRoomRequest{RuleId: "sichuan_xuezhandaodi_huansanzhang", DisplayName: "公开大厅桌"},
	}}
	writeEnv(t, creator, protocol.CreateRoomReq, create)
	createResp := readEnv(t, creator, protocol.CreateRoomResp).GetCreateRoomResp()
	require.Empty(t, createResp.GetErrorMessage())
	require.NotEmpty(t, createResp.GetRoomId())
	require.EqualValues(t, 0, createResp.GetSeatIndex())

	list := &clientv1.Envelope{ReqId: "list", Body: &clientv1.Envelope_ListRoomsReq{ListRoomsReq: &clientv1.ListRoomsRequest{PageSize: 20}}}
	writeEnv(t, creator, protocol.ListRoomsReq, list)
	listResp := readEnv(t, creator, protocol.ListRoomsResp).GetListRoomsResp()
	require.Empty(t, listResp.GetErrorMessage())
	require.Len(t, listResp.GetRooms(), 1)
	require.Equal(t, createResp.GetRoomId(), listResp.GetRooms()[0].GetRoomId())
	require.EqualValues(t, 1, listResp.GetRooms()[0].GetSeatCount())

	matcher := dialWS(t, srv)
	loginOnly(t, matcher, "matcher")
	match := &clientv1.Envelope{ReqId: "match", IdempotencyKey: "idem-match", Body: &clientv1.Envelope_AutoMatchReq{
		AutoMatchReq: &clientv1.AutoMatchRequest{RuleId: "sichuan_xuezhandaodi_huansanzhang"},
	}}
	writeEnv(t, matcher, protocol.AutoMatchReq, match)
	matchResp := readEnv(t, matcher, protocol.AutoMatchResp).GetAutoMatchResp()
	require.Empty(t, matchResp.GetErrorMessage())
	require.Equal(t, createResp.GetRoomId(), matchResp.GetRoomId())
	require.EqualValues(t, 1, matchResp.GetSeatIndex())
}

func TestWebSocketAutoMatchWithBotsReadyEmitsOpeningEvents(t *testing.T) {
	t.Parallel()
	roomRegistry := roomsvc.NewRoomRegistry()
	roomService := roomsvc.NewService(roomRegistry)
	hub := session.NewHub()
	srv := wsTestServer(t, Deps{Rooms: localgateway.NewLocalRoomGateway(roomService, hub, nil), Hub: hub})

	conn := dialWS(t, srv)
	loginOnly(t, conn, "matcher")
	match := &clientv1.Envelope{ReqId: "match-bots", IdempotencyKey: "idem-match-bots", Body: &clientv1.Envelope_AutoMatchReq{
		AutoMatchReq: &clientv1.AutoMatchRequest{RuleId: "sichuan_xuezhandaodi_huansanzhang", PadWithBots: true},
	}}
	writeEnv(t, conn, protocol.AutoMatchReq, match)
	matchResp := readEnv(t, conn, protocol.AutoMatchResp).GetAutoMatchResp()
	require.Empty(t, matchResp.GetErrorMessage())
	require.Len(t, matchResp.GetSeats(), 4)

	ready := &clientv1.Envelope{ReqId: "ready-after-bots", IdempotencyKey: "idem-ready-after-bots", Body: &clientv1.Envelope_ReadyReq{ReadyReq: &clientv1.ReadyRequest{}}}
	writeEnv(t, conn, protocol.ReadyReq, ready)
	readyResp := readEnv(t, conn, protocol.ReadyResp).GetReadyResp()
	require.Equal(t, clientv1.ErrorCode_ERROR_CODE_UNSPECIFIED, readyResp.GetErrorCode())

	require.NotNil(t, readUntilEnv(t, conn, protocol.InitialDealNotify, 8).GetInitialDeal())
	require.Equal(t, "exchange_three", readUntilEnv(t, conn, protocol.ActionNotify, 8).GetAction().GetAction())
}

func TestWebSocketPrivateRoomHiddenFromList(t *testing.T) {
	t.Parallel()
	roomRegistry := roomsvc.NewRoomRegistry()
	roomService := roomsvc.NewService(roomRegistry)
	hub := session.NewHub()
	srv := wsTestServer(t, Deps{Rooms: localgateway.NewLocalRoomGateway(roomService, hub, nil), Hub: hub})

	conn := dialWS(t, srv)
	loginOnly(t, conn, "private-creator")
	create := &clientv1.Envelope{ReqId: "create-private", IdempotencyKey: "idem-private", Body: &clientv1.Envelope_CreateRoomReq{
		CreateRoomReq: &clientv1.CreateRoomRequest{DisplayName: "私密桌", Private: true},
	}}
	writeEnv(t, conn, protocol.CreateRoomReq, create)
	require.NotEmpty(t, readEnv(t, conn, protocol.CreateRoomResp).GetCreateRoomResp().GetRoomId())

	writeEnv(t, conn, protocol.ListRoomsReq, &clientv1.Envelope{ReqId: "list", Body: &clientv1.Envelope_ListRoomsReq{ListRoomsReq: &clientv1.ListRoomsRequest{}}})
	listResp := readEnv(t, conn, protocol.ListRoomsResp).GetListRoomsResp()
	require.Empty(t, listResp.GetErrorMessage())
	require.Empty(t, listResp.GetRooms())
}

func loginOnly(t *testing.T, conn *websocket.Conn, name string) {
	t.Helper()
	writeEnv(t, conn, protocol.LoginReq, &clientv1.Envelope{
		ReqId: "login-" + name,
		Body:  &clientv1.Envelope_LoginReq{LoginReq: &clientv1.LoginRequest{Nickname: name}},
	})
	loginResp := readEnv(t, conn, protocol.LoginResp).GetLoginResp()
	require.NotEmpty(t, loginResp.GetUserId())
}

func writeEnv(t *testing.T, conn *websocket.Conn, msgID uint16, env *clientv1.Envelope) {
	t.Helper()
	payload, err := proto.Marshal(env)
	require.NoError(t, err)
	enc, _ := protocol.Encode(msgID, payload)
	require.NoError(t, conn.WriteMessage(websocket.BinaryMessage, enc))
}
