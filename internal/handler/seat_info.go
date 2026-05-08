package handler

import (
	"context"

	clientv1 "racoo.cn/lsp/api/gen/go/client/v1"
)

func selfSeatInfo(ctx context.Context, deps Deps, seat int32, userID string) []*clientv1.SeatInfo {
	seats := make([]*clientv1.SeatInfo, 0, 4)
	for i := int32(0); i < 4; i++ {
		info := &clientv1.SeatInfo{SeatIndex: i, Status: "empty"}
		if i == seat {
			info.UserId = userID
			info.Nickname = nicknameForUser(ctx, deps, userID)
			info.Online = userID != ""
			if info.Online {
				info.Status = "online"
			}
		}
		seats = append(seats, info)
	}
	return seats
}

func nicknameForUser(ctx context.Context, deps Deps, userID string) string {
	if deps.Users == nil || userID == "" {
		return ""
	}
	profile, ok, err := deps.Users.Get(ctx, userID)
	if err != nil || !ok {
		return ""
	}
	return profile.Nickname
}
