package roomadapter

import (
	"context"

	"google.golang.org/protobuf/proto"
	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	roomsvc "racoo.cn/lsp/internal/service/room"
	"racoo.cn/lsp/internal/store/redis"
	"racoo.cn/lsp/pkg/logx"
)

func (s *GRPCServer) persistRoomMeta(ctx context.Context, roomID string, seq int64, notification *roomsvc.Notification) {
	if s == nil || s.rdb == nil {
		return
	}
	var prev redis.RoomSnapMeta
	if meta, ok, err := s.rdb.GetRoomSnapMeta(ctx, roomID); err == nil && ok {
		prev = meta
	}
	players, state, _, ok := s.rooms.RoomSnapshot(roomID)
	if !ok {
		state = ""
	}
	if seq == 0 && s.ev != nil {
		if maxSeq, err := s.ev.MaxSeq(ctx, roomID); err == nil {
			seq = maxSeq
		}
	}
	meta := redis.RoomSnapMeta{
		Seq:       seq,
		PlayerIDs: append([]string(nil), players...),
		State:     state,
		QueSuits:  append([]int32(nil), prev.QueSuits...),
	}
	if notification != nil {
		if qs := queSuitsFromNotification(*notification); len(qs) > 0 {
			meta.QueSuits = qs
		}
	}
	if roundJSON, err := s.rooms.RoundPersistSnapshot(ctx, roomID); err == nil && len(roundJSON) > 0 {
		meta.RoundJSON = string(roundJSON)
	}
	if err := s.rdb.PutRoomSnapMeta(ctx, roomID, meta, 0); err != nil {
		logx.Warn(logx.WithRoomID(ctx, roomID), "房间快照元数据写入 Redis 失败", "err", err.Error())
	}
}

func queSuitsFromNotification(n roomsvc.Notification) []int32 {
	if n.Kind != roomsvc.KindOpeningDone {
		return nil
	}
	var env clientv1.Envelope
	if err := proto.Unmarshal(n.Payload, &env); err != nil {
		return nil
	}
	return openingSeatInts(env.GetOpeningDone(), "que_suit")
}

func openingSeatInts(done *clientv1.OpeningDoneNotify, key string) []int32 {
	if done == nil {
		return nil
	}
	for _, group := range done.GetSeatInts() {
		if group.GetKey() == key {
			return append([]int32(nil), group.GetValues()...)
		}
	}
	return nil
}

func pickLastQueSuits(ctx context.Context, s *GRPCServer, roomID string) []int32 {
	if s.ev == nil {
		return nil
	}
	rows, err := s.ev.ListEventsAfter(ctx, roomID, 0)
	if err != nil {
		return nil
	}
	for i := len(rows) - 1; i >= 0; i-- {
		if rows[i].Kind != string(roomsvc.KindOpeningDone) {
			continue
		}
		var env clientv1.Envelope
		if err := proto.Unmarshal(rows[i].Payload, &env); err != nil {
			return nil
		}
		if qs := openingSeatInts(env.GetOpeningDone(), "que_suit"); len(qs) > 0 {
			return qs
		}
	}
	return nil
}
