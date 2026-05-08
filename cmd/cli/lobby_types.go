package main

import "context"

// LobbyGateway 抽象大厅场景对网络层的所有依赖。
type LobbyGateway interface {
	LeaveRoom(ctx context.Context) error
	ListRules(ctx context.Context) ([]LobbyRuleMeta, error)
	AutoMatch(ctx context.Context, ruleID string) (LobbyJoinResult, error)
	ListRooms(ctx context.Context, pageToken string) (LobbyRoomList, error)
	CreateRoom(ctx context.Context, opts LobbyCreateOpts) (LobbyJoinResult, error)
	JoinRoom(ctx context.Context, roomID string) (LobbyJoinResult, error)
	ChangeNickname(name string)
}

// LobbyJoinResult 描述一次成功进入房间的最小信息。
type LobbyJoinResult struct {
	RoomID      string
	SeatIndex   int32
	DisplayName string
	RuleID      string
}

// LobbyRoomMeta 是公开房间列表中每条房间的玩家可读视图。
type LobbyRoomMeta struct {
	RoomID      string
	DisplayName string
	Players     int
	Capacity    int
	RuleID      string
}

// LobbyRoomList 一页房间列表，支持游标分页。
type LobbyRoomList struct {
	Rooms         []LobbyRoomMeta
	NextPageToken string
}

// LobbyRuleMeta 是后端规则元数据的玩家可读投影。
type LobbyRuleMeta struct {
	RuleID          string
	DisplayName     string
	ShortDesc       string
	EnabledFeatures []string
	MaxHands        int32
}

// LobbyCreateOpts 创建房间的玩家选项。
type LobbyCreateOpts struct {
	RuleID      string
	DisplayName string
	Private     bool
}

func displayRoomName(meta LobbyRoomMeta) string {
	if meta.DisplayName != "" {
		return meta.DisplayName
	}
	return meta.RoomID
}

func displayRuleName(meta LobbyRuleMeta) string {
	if meta.DisplayName != "" {
		return meta.DisplayName
	}
	return meta.RuleID
}

func defaultRoomDisplayName(rule LobbyRuleMeta) string {
	name := displayRuleName(rule)
	if name == "" {
		return "麻将房间"
	}
	return name + " 房间"
}
