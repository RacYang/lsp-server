package roomadapter

import (
	"context"

	eng "racoo.cn/lsp/internal/service/room/engine"

	"google.golang.org/protobuf/proto"
	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
	"racoo.cn/lsp/internal/store/redis"
	"racoo.cn/lsp/pkg/logx"
)

// persistRoomMeta 把房间当前内存态的一致切片写入 Redis snapmeta，供节点重启恢复使用。
//
// 不变量：snapmeta 只能整体派生自一次成功读取的内存切片。任何来源（上一份元数据、
// 房间快照、局内快照）读取失败时必须放弃本次写入，保留上一份一致快照——
// 把"读取失败"当成空值写入会破坏性覆盖好快照，导致恢复时整局甚至整房间丢失。
// "局已结束"（RoundPersistSnapshot 返回空且无错误）才是清空 RoundJSON 的唯一合法路径。
//
// PG 事件流是权威数据源，snapmeta 是恢复加速器；PutRoomSnapMeta 写入失败只告警，
// 由下一事件重写补齐。
func (s *GRPCServer) persistRoomMeta(ctx context.Context, roomID string, seq int64, notifications []eng.Notification) {
	if s == nil || s.rdb == nil {
		return
	}
	ctx = logx.WithRoomID(ctx, roomID)
	prev, _, err := s.rdb.GetRoomSnapMeta(ctx, roomID)
	if err != nil {
		logx.Error(ctx, "读取上一份房间快照元数据失败，跳过本次快照写入", "err", err.Error())
		return
	}
	players, state, _, ok := s.rooms.RoomSnapshot(roomID)
	if !ok {
		logx.Error(ctx, "读取房间内存快照失败，跳过本次快照写入")
		return
	}
	roundJSON, err := s.rooms.RoundPersistSnapshot(ctx, roomID)
	if err != nil {
		logx.Error(ctx, "读取局内持久化快照失败，跳过本次快照写入", "err", err.Error())
		return
	}
	if seq == 0 && s.ev != nil {
		maxSeq, err := s.ev.MaxSeq(ctx, roomID)
		if err != nil {
			logx.Warn(ctx, "查询事件最大序号失败，沿用上一份快照序号", "err", err.Error())
			maxSeq = prev.Seq
		}
		seq = maxSeq
	}
	if seq < prev.Seq {
		// Seq 与 PG 事件序号对齐且单调不减，快照序号不得回退。
		seq = prev.Seq
	}
	meta := redis.RoomSnapMeta{
		Seq:       seq,
		PlayerIDs: append([]string(nil), players...),
		State:     state,
		QueSuits:  append([]int32(nil), prev.QueSuits...),
	}
	if qs := queSuitsFromNotifications(notifications); len(qs) > 0 {
		meta.QueSuits = qs
	}
	if len(roundJSON) > 0 {
		meta.RoundJSON = string(roundJSON)
	}
	if err := s.rdb.PutRoomSnapMeta(ctx, roomID, meta, 0); err != nil {
		logx.Warn(ctx, "房间快照元数据写入 Redis 失败", "err", err.Error())
	}
}

// queSuitsFromNotifications 从一批通知中提取最后一次定缺结果；无定缺事件时返回 nil。
func queSuitsFromNotifications(notifications []eng.Notification) []int32 {
	for i := len(notifications) - 1; i >= 0; i-- {
		if qs := queSuitsFromNotification(notifications[i]); len(qs) > 0 {
			return qs
		}
	}
	return nil
}

func queSuitsFromNotification(n eng.Notification) []int32 {
	if n.Kind != eng.KindOpeningDone {
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
		if rows[i].Kind != string(eng.KindOpeningDone) {
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
