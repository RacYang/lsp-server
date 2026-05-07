package main

import (
	"context"
	"errors"
	"io"
	"strconv"
	"strings"
)

// LobbyGateway 抽象 lobby 主循环对网络层的所有依赖。
//
// 这里所有方法都是同步的「发请求 + 等响应」：调用返回时要么拿到结果，
// 要么带回错误（含取消、超时、协议错误）。把"等待响应"封装在 gateway 里，
// 主循环就能保持单线程的传统命令行节奏。
type LobbyGateway interface {
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

// LobbyCreateOpts 创建房间的玩家选项。
type LobbyCreateOpts struct {
	RuleID      string
	DisplayName string
	Private     bool
}

// LobbyExitReason 指示 lobby 主循环退出的原因，决定外层是进入牌桌还是结束进程。
type LobbyExitReason int

const (
	// LobbyExitQuit 玩家选择"退出游戏"或读到 EOF。
	LobbyExitQuit LobbyExitReason = iota
	// LobbyExitJoinRoom 成功进入房间，外层应切换到牌桌全屏。
	LobbyExitJoinRoom
)

// LobbyOutcome 主循环返回值，封装下一步动作所需的全部数据。
type LobbyOutcome struct {
	Reason     LobbyExitReason
	JoinResult LobbyJoinResult
}

// RunLobby 是 lobby 阶段的主循环：打印菜单 → 读输入 → 调度 handler → 重复。
//
// cfg 会在玩家修改昵称时被就地写入，调用方负责按需持久化（一般是 SaveConfig）。
// 当 handler 成功使玩家进入房间时立即返回 LobbyExitJoinRoom，
// 让外层切换到牌桌全屏；否则一直循环直到玩家选择退出或上下文取消。
func RunLobby(ctx context.Context, p Prompter, gw LobbyGateway, cfg *Config) (LobbyOutcome, error) {
	if cfg == nil {
		return LobbyOutcome{}, errors.New("配置指针为空")
	}
	if cfg.Nickname != "" {
		p.Printf("欢迎, %s", cfg.Nickname)
		p.PrintBlank()
	}
	for ctx.Err() == nil {
		printLobbyStatus(p, cfg)
		printMainMenu(p)
		raw, err := p.AskLine("> ")
		if err != nil {
			if errors.Is(err, io.EOF) {
				return LobbyOutcome{Reason: LobbyExitQuit}, nil
			}
			return LobbyOutcome{}, err
		}
		choice := strings.TrimSpace(raw)
		switch choice {
		case "":
			p.PrintBlank()
		case "1":
			res, err := handleAutoMatch(ctx, p, gw)
			if err != nil {
				p.Printf("匹配失败: %v", err)
				p.PrintBlank()
				continue
			}
			return LobbyOutcome{Reason: LobbyExitJoinRoom, JoinResult: res}, nil
		case "2":
			res, ok, err := handleListRoomsAndJoin(ctx, p, gw)
			if err != nil {
				p.Printf("操作失败: %v", err)
				p.PrintBlank()
				continue
			}
			if ok {
				return LobbyOutcome{Reason: LobbyExitJoinRoom, JoinResult: res}, nil
			}
		case "3":
			res, err := handleCreateRoom(ctx, p, gw)
			if err != nil {
				p.Printf("创建房间失败: %v", err)
				p.PrintBlank()
				continue
			}
			return LobbyOutcome{Reason: LobbyExitJoinRoom, JoinResult: res}, nil
		case "4":
			res, ok, err := handleJoinByCode(ctx, p, gw)
			if err != nil {
				p.Printf("加入失败: %v", err)
				p.PrintBlank()
				continue
			}
			if ok {
				return LobbyOutcome{Reason: LobbyExitJoinRoom, JoinResult: res}, nil
			}
		case "5":
			handleChangeNickname(p, gw, cfg)
		case "s", "S":
			handleSettings(p, cfg)
		case "t", "T":
			printTutorial(p)
		case "q", "Q", "quit", "exit":
			p.Print("再见!")
			return LobbyOutcome{Reason: LobbyExitQuit}, nil
		default:
			p.Printf("未知选项 %q,请输入 1-5、s、t 或 q", choice)
			p.PrintBlank()
		}
	}
	return LobbyOutcome{Reason: LobbyExitQuit}, ctx.Err()
}

func printMainMenu(p Prompter) {
	p.Print("请选择:")
	p.Print("  1) 快速开始 (人少自动加机器人)")
	p.Print("  2) 查看公开房间")
	p.Print("  3) 创建房间")
	p.Print("  4) 输入房间码加入")
	p.Print("  5) 修改昵称")
	p.Print("  s) 设置")
	p.Print("  t) 玩法说明")
	p.Print("  q) 退出")
}

func printLobbyStatus(p Prompter, cfg *Config) {
	server := cfg.ServerURL
	if server == "" {
		server = defaultServerURL
	}
	name := cfg.Nickname
	if name == "" {
		name = "(未设置)"
	}
	p.Printf("状态: 服务器 %s | 昵称 %s", server, name)
}

func handleAutoMatch(ctx context.Context, p Prompter, gw LobbyGateway) (LobbyJoinResult, error) {
	p.Print("正在匹配,按 Ctrl+C 取消...")
	res, err := gw.AutoMatch(ctx, "")
	if err != nil {
		return LobbyJoinResult{}, err
	}
	p.Printf("匹配成功,进入房间 %s (座位 %d)", displayRoomID(res), res.SeatIndex+1)
	return res, nil
}

func handleListRoomsAndJoin(ctx context.Context, p Prompter, gw LobbyGateway) (LobbyJoinResult, bool, error) {
	list, err := gw.ListRooms(ctx, "")
	if err != nil {
		return LobbyJoinResult{}, false, err
	}
	if len(list.Rooms) == 0 {
		p.Print("当前没有公开房间,试试 1) 快速开始 或 3) 创建房间")
		return LobbyJoinResult{}, false, nil
	}
	p.Print("公开房间:")
	for i, r := range list.Rooms {
		p.Printf("  %d) %s  规则:%s  人数:%d/%d", i+1, displayRoomName(r), r.RuleID, r.Players, r.Capacity)
	}
	p.Print("  0) 返回主菜单")
	raw, err := p.AskLine("选择房间编号 > ")
	if err != nil {
		return LobbyJoinResult{}, false, err
	}
	choice := strings.TrimSpace(raw)
	if choice == "" || choice == "0" {
		return LobbyJoinResult{}, false, nil
	}
	n, err := strconv.Atoi(choice)
	if err != nil || n < 1 || n > len(list.Rooms) {
		p.Printf("编号无效: %s", choice)
		return LobbyJoinResult{}, false, nil
	}
	target := list.Rooms[n-1]
	res, err := gw.JoinRoom(ctx, target.RoomID)
	if err != nil {
		return LobbyJoinResult{}, false, err
	}
	p.Printf("已加入房间 %s (座位 %d)", target.RoomID, res.SeatIndex+1)
	return res, true, nil
}

func handleCreateRoom(ctx context.Context, p Prompter, gw LobbyGateway) (LobbyJoinResult, error) {
	rule, err := p.AskLine("规则 (留空使用默认) > ")
	if err != nil {
		return LobbyJoinResult{}, err
	}
	name, err := p.AskLine("房间名称 (留空使用默认) > ")
	if err != nil {
		return LobbyJoinResult{}, err
	}
	private, err := askYesNo(p, "私密房间? (y/N)", false)
	if err != nil {
		return LobbyJoinResult{}, err
	}
	res, err := gw.CreateRoom(ctx, LobbyCreateOpts{
		RuleID:      strings.TrimSpace(rule),
		DisplayName: strings.TrimSpace(name),
		Private:     private,
	})
	if err != nil {
		return LobbyJoinResult{}, err
	}
	p.Printf("已创建房间 %s,你是座位 %d", displayRoomID(res), res.SeatIndex+1)
	if private {
		p.Printf("把房间码 %s 分享给朋友吧", res.RoomID)
	}
	return res, nil
}

func handleJoinByCode(ctx context.Context, p Prompter, gw LobbyGateway) (LobbyJoinResult, bool, error) {
	raw, err := p.AskLine("请输入房间码 > ")
	if err != nil {
		return LobbyJoinResult{}, false, err
	}
	code := strings.TrimSpace(raw)
	if code == "" {
		return LobbyJoinResult{}, false, nil
	}
	res, err := gw.JoinRoom(ctx, code)
	if err != nil {
		return LobbyJoinResult{}, false, err
	}
	p.Printf("已加入房间 %s (座位 %d)", code, res.SeatIndex+1)
	return res, true, nil
}

func handleChangeNickname(p Prompter, gw LobbyGateway, cfg *Config) {
	raw, err := p.AskLine("新昵称 (留空取消) > ")
	if err != nil {
		p.Printf("修改昵称失败: %v", err)
		return
	}
	name := strings.TrimSpace(raw)
	if name == "" {
		return
	}
	cfg.Nickname = name
	gw.ChangeNickname(name)
	p.Printf("昵称已更新为 %s", name)
}

func handleSettings(p Prompter, cfg *Config) {
	p.Print("设置:")
	p.Printf("  当前牌面主题: %s", ParseTileTheme(cfg.TileTheme).String())
	raw, err := p.AskLine("切换主题? (y/N) > ")
	if err != nil {
		p.Printf("设置失败: %v", err)
		return
	}
	if strings.EqualFold(strings.TrimSpace(raw), "y") || strings.EqualFold(strings.TrimSpace(raw), "yes") {
		theme := ParseTileTheme(cfg.TileTheme)
		if theme == TileThemeUnicode {
			cfg.TileTheme = TileThemeASCII.String()
		} else {
			cfg.TileTheme = TileThemeUnicode.String()
		}
		p.Printf("牌面主题已切换为 %s", cfg.TileTheme)
	}
}

func printTutorial(p Prompter) {
	p.Print("玩法说明:")
	p.Print("  目标: 缺一门后尽快胡牌,结算按番数计算。")
	p.Print("  开局: 换三张后选择定缺花色,本局不能保留该花色到胡牌。")
	p.Print("  牌桌: ←→ 选牌,Enter 出牌; 有碰/杠/胡窗口时按 p/g/h/n。")
	p.Print("  机器人: 快速开始会自动补齐,牌桌 waiting 阶段可按 b/B 补位。")
	p.Print("  离桌: 牌桌按 q 返回大厅; 断线后可用会话恢复。")
	p.PrintBlank()
}

func askYesNo(p Prompter, label string, defaultYes bool) (bool, error) {
	raw, err := p.AskLine(label + " > ")
	if err != nil {
		return false, err
	}
	answer := strings.ToLower(strings.TrimSpace(raw))
	if answer == "" {
		return defaultYes, nil
	}
	switch answer {
	case "y", "yes", "true", "1":
		return true, nil
	default:
		return false, nil
	}
}

func displayRoomID(res LobbyJoinResult) string {
	if res.DisplayName != "" {
		return res.DisplayName
	}
	return res.RoomID
}

func displayRoomName(meta LobbyRoomMeta) string {
	if meta.DisplayName != "" {
		return meta.DisplayName
	}
	return meta.RoomID
}
