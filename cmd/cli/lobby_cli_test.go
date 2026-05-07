package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeGateway 是 LobbyGateway 的内存实现，可按预设脚本依次回应或返回错误。
type fakeGateway struct {
	mu sync.Mutex

	autoMatch     func() (LobbyJoinResult, error)
	listRooms     func() (LobbyRoomList, error)
	createRoom    func(LobbyCreateOpts) (LobbyJoinResult, error)
	joinRoom      func(string) (LobbyJoinResult, error)
	nicknameCalls []string
}

// AutoMatch 把脚本里的 autoMatch 闭包透传出去,未配置时返回 not configured 让测试一眼看出漏配。
func (f *fakeGateway) AutoMatch(ctx context.Context, ruleID string) (LobbyJoinResult, error) {
	if f.autoMatch == nil {
		return LobbyJoinResult{}, errors.New("not configured")
	}
	return f.autoMatch()
}

// ListRooms 透传 listRooms 闭包,用于校验大厅列表与翻页交互。
func (f *fakeGateway) ListRooms(ctx context.Context, pageToken string) (LobbyRoomList, error) {
	if f.listRooms == nil {
		return LobbyRoomList{}, errors.New("not configured")
	}
	return f.listRooms()
}

// CreateRoom 透传 createRoom 闭包,允许测试断言传入的房间参数。
func (f *fakeGateway) CreateRoom(ctx context.Context, opts LobbyCreateOpts) (LobbyJoinResult, error) {
	if f.createRoom == nil {
		return LobbyJoinResult{}, errors.New("not configured")
	}
	return f.createRoom(opts)
}

// JoinRoom 透传 joinRoom 闭包,常用于"输入房间码加入"路径的覆盖。
func (f *fakeGateway) JoinRoom(ctx context.Context, roomID string) (LobbyJoinResult, error) {
	if f.joinRoom == nil {
		return LobbyJoinResult{}, errors.New("not configured")
	}
	return f.joinRoom(roomID)
}

// ChangeNickname 把改名调用累计在 nicknameCalls,测试据此断言"5) 改名"是否落地。
func (f *fakeGateway) ChangeNickname(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nicknameCalls = append(f.nicknameCalls, name)
}

// newPrompterFromInput 用预设输入行构造 Prompter,出参 strings.Builder 作为输出缓冲方便断言文本提示。
func newPrompterFromInput(lines []string) (Prompter, *strings.Builder) {
	in := strings.NewReader(strings.Join(lines, "\n") + "\n")
	out := &strings.Builder{}
	return NewIOPrompter(in, out), out
}

func TestRunLobbyQuit(t *testing.T) {
	p, out := newPrompterFromInput([]string{"q"})
	cfg := &Config{Nickname: "racoo"}
	outcome, err := RunLobby(context.Background(), p, &fakeGateway{}, cfg)
	require.NoError(t, err)
	require.Equal(t, LobbyExitQuit, outcome.Reason)
	require.Contains(t, out.String(), "请选择")
	require.Contains(t, out.String(), "再见")
}

func TestRunLobbyAutoMatchSuccess(t *testing.T) {
	p, out := newPrompterFromInput([]string{"1"})
	gw := &fakeGateway{
		autoMatch: func() (LobbyJoinResult, error) {
			return LobbyJoinResult{RoomID: "ABCD", SeatIndex: 2}, nil
		},
	}
	outcome, err := RunLobby(context.Background(), p, gw, &Config{Nickname: "x"})
	require.NoError(t, err)
	require.Equal(t, LobbyExitJoinRoom, outcome.Reason)
	require.Equal(t, "ABCD", outcome.JoinResult.RoomID)
	require.Contains(t, out.String(), "正在匹配")
	require.Contains(t, out.String(), "进入房间 ABCD")
}

func TestRunLobbyAutoMatchFailureKeepsLoop(t *testing.T) {
	p, out := newPrompterFromInput([]string{"1", "q"})
	gw := &fakeGateway{
		autoMatch: func() (LobbyJoinResult, error) {
			return LobbyJoinResult{}, errors.New("匹配池为空")
		},
	}
	outcome, err := RunLobby(context.Background(), p, gw, &Config{})
	require.NoError(t, err)
	require.Equal(t, LobbyExitQuit, outcome.Reason)
	require.Contains(t, out.String(), "匹配失败")
}

func TestRunLobbyTutorialThenQuit(t *testing.T) {
	p, out := newPrompterFromInput([]string{"t", "q"})
	outcome, err := RunLobby(context.Background(), p, &fakeGateway{}, &Config{})
	require.NoError(t, err)
	require.Equal(t, LobbyExitQuit, outcome.Reason)
	require.Contains(t, out.String(), "玩法说明")
	require.Contains(t, out.String(), "牌桌: ←→ 选牌")
}

func TestRunLobbyListRoomsAndJoinByIndex(t *testing.T) {
	p, _ := newPrompterFromInput([]string{"2", "1"})
	gw := &fakeGateway{
		listRooms: func() (LobbyRoomList, error) {
			return LobbyRoomList{Rooms: []LobbyRoomMeta{
				{RoomID: "R1", DisplayName: "Alice 的局", Players: 1, Capacity: 4, RuleID: "scmj"},
			}}, nil
		},
		joinRoom: func(id string) (LobbyJoinResult, error) {
			require.Equal(t, "R1", id)
			return LobbyJoinResult{RoomID: id, SeatIndex: 0}, nil
		},
	}
	outcome, err := RunLobby(context.Background(), p, gw, &Config{})
	require.NoError(t, err)
	require.Equal(t, LobbyExitJoinRoom, outcome.Reason)
	require.Equal(t, "R1", outcome.JoinResult.RoomID)
}

func TestRunLobbyListRoomsBackToMenu(t *testing.T) {
	p, out := newPrompterFromInput([]string{"2", "0", "q"})
	gw := &fakeGateway{
		listRooms: func() (LobbyRoomList, error) {
			return LobbyRoomList{Rooms: []LobbyRoomMeta{{RoomID: "R1", DisplayName: "局 1", Capacity: 4}}}, nil
		},
	}
	outcome, err := RunLobby(context.Background(), p, gw, &Config{})
	require.NoError(t, err)
	require.Equal(t, LobbyExitQuit, outcome.Reason)
	require.Contains(t, out.String(), "公开房间")
}

func TestRunLobbyEmptyRoomListShowsHint(t *testing.T) {
	p, out := newPrompterFromInput([]string{"2", "q"})
	gw := &fakeGateway{
		listRooms: func() (LobbyRoomList, error) { return LobbyRoomList{}, nil },
	}
	_, err := RunLobby(context.Background(), p, gw, &Config{})
	require.NoError(t, err)
	require.Contains(t, out.String(), "当前没有公开房间")
}

func TestRunLobbyCreateRoomPrivate(t *testing.T) {
	p, out := newPrompterFromInput([]string{"3", "scmj", "我的私密局", "y"})
	gw := &fakeGateway{
		createRoom: func(opts LobbyCreateOpts) (LobbyJoinResult, error) {
			require.Equal(t, "scmj", opts.RuleID)
			require.Equal(t, "我的私密局", opts.DisplayName)
			require.True(t, opts.Private)
			return LobbyJoinResult{RoomID: "R7K2", SeatIndex: 0, DisplayName: opts.DisplayName}, nil
		},
	}
	outcome, err := RunLobby(context.Background(), p, gw, &Config{})
	require.NoError(t, err)
	require.Equal(t, LobbyExitJoinRoom, outcome.Reason)
	require.Equal(t, "R7K2", outcome.JoinResult.RoomID)
	require.Contains(t, out.String(), "把房间码 R7K2 分享给朋友吧")
}

func TestRunLobbyJoinByCode(t *testing.T) {
	p, out := newPrompterFromInput([]string{"4", "ABCD"})
	gw := &fakeGateway{
		joinRoom: func(id string) (LobbyJoinResult, error) {
			return LobbyJoinResult{RoomID: id, SeatIndex: 1}, nil
		},
	}
	outcome, err := RunLobby(context.Background(), p, gw, &Config{})
	require.NoError(t, err)
	require.Equal(t, "ABCD", outcome.JoinResult.RoomID)
	require.Contains(t, out.String(), "已加入房间 ABCD")
}

func TestRunLobbyJoinByCodeBlankReturns(t *testing.T) {
	p, _ := newPrompterFromInput([]string{"4", "", "q"})
	gw := &fakeGateway{}
	outcome, err := RunLobby(context.Background(), p, gw, &Config{})
	require.NoError(t, err)
	require.Equal(t, LobbyExitQuit, outcome.Reason)
}

func TestRunLobbyChangeNicknameUpdatesConfig(t *testing.T) {
	p, _ := newPrompterFromInput([]string{"5", "carol", "q"})
	cfg := &Config{Nickname: "old"}
	gw := &fakeGateway{}
	_, err := RunLobby(context.Background(), p, gw, cfg)
	require.NoError(t, err)
	require.Equal(t, "carol", cfg.Nickname)
	require.Equal(t, []string{"carol"}, gw.nicknameCalls)
}

func TestRunLobbyUnknownChoiceShowsHint(t *testing.T) {
	p, out := newPrompterFromInput([]string{"x", "q"})
	gw := &fakeGateway{}
	_, err := RunLobby(context.Background(), p, gw, &Config{})
	require.NoError(t, err)
	require.Contains(t, out.String(), "未知选项")
}

func TestRunLobbyEOFExitsCleanly(t *testing.T) {
	p, _ := newPrompterFromInput(nil)
	_, err := RunLobby(context.Background(), p, &fakeGateway{}, &Config{})
	require.NoError(t, err)
}

func TestRunLobbyContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := NewIOPrompter(strings.NewReader(""), &strings.Builder{})
	_, err := RunLobby(ctx, p, &fakeGateway{}, &Config{})
	require.ErrorIs(t, err, context.Canceled)
}

func TestRunLobbyConfigNilReturnsError(t *testing.T) {
	p, _ := newPrompterFromInput([]string{"q"})
	_, err := RunLobby(context.Background(), p, &fakeGateway{}, nil)
	require.Error(t, err)
}

// 占位引用,确认 fmt 包始终被使用,避免删除任意一个 t.Logf 后 imports 漂移。
var _ = fmt.Sprintf
