package handler

import (
	"errors"
	"strings"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/net/msgid"
	roomsvc "racoo.cn/lsp/internal/service/room"
)

func discardErrEnvelope(reqID string, after func(), err error) (*clientv1.Envelope, func()) {
	if err != nil {
		return &clientv1.Envelope{ReqId: reqID, Body: &clientv1.Envelope_DiscardResp{DiscardResp: &clientv1.DiscardResponse{
			ErrorCode:    actionErrorCode(err),
			ErrorMessage: err.Error(),
		}}}, nil
	}
	return &clientv1.Envelope{ReqId: reqID, Body: &clientv1.Envelope_DiscardResp{DiscardResp: &clientv1.DiscardResponse{}}}, after
}

func pongErrEnvelope(reqID string, after func(), err error) (*clientv1.Envelope, func()) {
	if err != nil {
		return &clientv1.Envelope{ReqId: reqID, Body: &clientv1.Envelope_PongResp{PongResp: &clientv1.PongResponse{
			ErrorCode:    actionErrorCode(err),
			ErrorMessage: err.Error(),
		}}}, nil
	}
	return &clientv1.Envelope{ReqId: reqID, Body: &clientv1.Envelope_PongResp{PongResp: &clientv1.PongResponse{}}}, after
}

func gangErrEnvelope(reqID string, after func(), err error) (*clientv1.Envelope, func()) {
	if err != nil {
		return &clientv1.Envelope{ReqId: reqID, Body: &clientv1.Envelope_GangResp{GangResp: &clientv1.GangResponse{
			ErrorCode:    actionErrorCode(err),
			ErrorMessage: err.Error(),
		}}}, nil
	}
	return &clientv1.Envelope{ReqId: reqID, Body: &clientv1.Envelope_GangResp{GangResp: &clientv1.GangResponse{}}}, after
}

func huErrEnvelope(reqID string, after func(), err error) (*clientv1.Envelope, func()) {
	if err != nil {
		return &clientv1.Envelope{ReqId: reqID, Body: &clientv1.Envelope_HuResp{HuResp: &clientv1.HuResponse{
			ErrorCode:    actionErrorCode(err),
			ErrorMessage: err.Error(),
		}}}, nil
	}
	return &clientv1.Envelope{ReqId: reqID, Body: &clientv1.Envelope_HuResp{HuResp: &clientv1.HuResponse{}}}, after
}

func exchangeThreeErrEnvelope(reqID string, after func(), err error) (*clientv1.Envelope, func()) {
	if err != nil {
		return &clientv1.Envelope{ReqId: reqID, Body: &clientv1.Envelope_ExchangeThreeResp{ExchangeThreeResp: &clientv1.ExchangeThreeResponse{
			ErrorCode:    actionErrorCode(err),
			ErrorMessage: err.Error(),
		}}}, nil
	}
	return &clientv1.Envelope{ReqId: reqID, Body: &clientv1.Envelope_ExchangeThreeResp{ExchangeThreeResp: &clientv1.ExchangeThreeResponse{}}}, after
}

func queMenErrEnvelope(reqID string, after func(), err error) (*clientv1.Envelope, func()) {
	if err != nil {
		return &clientv1.Envelope{ReqId: reqID, Body: &clientv1.Envelope_QueMenResp{QueMenResp: &clientv1.QueMenResponse{
			ErrorCode:    actionErrorCode(err),
			ErrorMessage: err.Error(),
		}}}, nil
	}
	return &clientv1.Envelope{ReqId: reqID, Body: &clientv1.Envelope_QueMenResp{QueMenResp: &clientv1.QueMenResponse{}}}, after
}

func actionErrorCode(err error) clientv1.ErrorCode {
	if errors.Is(err, roomsvc.ErrRateLimited) {
		rateLimitedTotal.WithLabelValues("room").Inc()
		return clientv1.ErrorCode_ERROR_CODE_RATE_LIMITED
	}
	return clientv1.ErrorCode_ERROR_CODE_INVALID_STATE
}

// joinRoomErrorCode 将进房失败映射为客户端 ErrorCode；未知错误使用 UNSPECIFIED，避免误报「房间已满」。
func joinRoomErrorCode(err error) clientv1.ErrorCode {
	if err == nil {
		return clientv1.ErrorCode_ERROR_CODE_UNSPECIFIED
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "room full"):
		return clientv1.ErrorCode_ERROR_CODE_ROOM_FULL
	case strings.Contains(msg, "room not found"):
		return clientv1.ErrorCode_ERROR_CODE_ROOM_NOT_FOUND
	case strings.Contains(msg, "invalid argument"):
		return clientv1.ErrorCode_ERROR_CODE_INVALID_STATE
	default:
		return clientv1.ErrorCode_ERROR_CODE_UNSPECIFIED
	}
}

func outboundMsgID(kind roomsvc.Kind) (uint16, bool) {
	switch kind {
	case roomsvc.KindExchangeThreeDone:
		return msgid.ExchangeThreeDone, true
	case roomsvc.KindQueMenDone:
		return msgid.QueMenDone, true
	case roomsvc.KindStartGame:
		return msgid.StartGame, true
	case roomsvc.KindDrawTile:
		return msgid.DrawTile, true
	case roomsvc.KindAction:
		return msgid.ActionNotify, true
	case roomsvc.KindSettlement:
		return msgid.Settlement, true
	default:
		return 0, false
	}
}
