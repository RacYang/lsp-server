package main

import (
	"errors"

	"github.com/gdamore/tcell/v2"
)

// ScreenFactory 抽象 tcell 屏幕创建，便于测试用 SimulationScreen 替换真实 TTY。
type ScreenFactory func() (tcell.Screen, error)

// defaultScreenFactory 在生产模式下创建并初始化默认 tcell 屏幕。
func defaultScreenFactory() (tcell.Screen, error) {
	return tcell.NewScreen()
}

// TerminalSwitch 在 lobby 行式模式与牌桌全屏模式之间切换终端控制权。
//
// 切换的语义边界：
//   - lobby 模式由调用方直接使用 stdin/stdout，TerminalSwitch 不接管。
//   - 牌桌模式通过 EnterFullscreen 接管 alternate screen；牌桌结束后必须
//     调用 LeaveFullscreen 才能恢复 lobby 行式输出。
//
// 这套显式生命周期协议避免了 bufio 预读缓冲跨切换误读的隐患。
type TerminalSwitch struct {
	factory ScreenFactory
	current tcell.Screen
}

// NewTerminalSwitch 返回生产环境用的切换器，使用真实 tcell 屏幕。
func NewTerminalSwitch() *TerminalSwitch {
	return &TerminalSwitch{factory: defaultScreenFactory}
}

// NewTerminalSwitchWithFactory 允许调用方注入自定义工厂，主要供测试使用。
func NewTerminalSwitchWithFactory(factory ScreenFactory) *TerminalSwitch {
	return &TerminalSwitch{factory: factory}
}

// EnterFullscreen 进入 alternate screen 并返回可绘制的 tcell.Screen。
//
// 重复进入会立即返回错误而不是覆盖前一个屏幕，便于尽早暴露调用顺序问题。
func (t *TerminalSwitch) EnterFullscreen() (tcell.Screen, error) {
	if t.current != nil {
		return nil, errors.New("已经处于全屏模式")
	}
	scr, err := t.factory()
	if err != nil {
		return nil, err
	}
	if err := scr.Init(); err != nil {
		return nil, err
	}
	t.current = scr
	return scr, nil
}

// LeaveFullscreen 退出 alternate screen 并释放 tcell 资源。
//
// 当前未处于全屏模式时是安全 no-op，便于在错误恢复路径中无脑调用。
func (t *TerminalSwitch) LeaveFullscreen() {
	if t.current == nil {
		return
	}
	t.current.Fini()
	t.current = nil
}

// IsFullscreen 返回是否当前处于全屏模式。
func (t *TerminalSwitch) IsFullscreen() bool {
	return t.current != nil
}
