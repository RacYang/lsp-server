package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type UI struct {
	app        *tview.Application
	pages      *tview.Pages
	board      *tview.TextView
	lobbyTable *tview.Table
	loginForm  *tview.Form
	lobbyInput *tview.InputField
	tableInput *tview.InputField
	state      *AppState
	client     *WSClient
	handler    *CommandHandler
	opts       RenderOptions
	runCtx     context.Context
	startMu    sync.Mutex
	started    bool
}

func NewUI(state *AppState, client *WSClient, handler *CommandHandler, opts RenderOptions) *UI {
	app := tview.NewApplication()
	board := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(false).
		SetWrap(false)
	lobbyTable := tview.NewTable().SetSelectable(true, false)
	loginForm := tview.NewForm()
	ui := &UI{app: app, pages: tview.NewPages(), board: board, lobbyTable: lobbyTable, loginForm: loginForm, state: state, client: client, handler: handler, opts: opts}
	ui.lobbyInput = ui.newCommandInput("大厅> ")
	ui.tableInput = ui.newCommandInput("> ")
	ui.configureLoginForm()
	lobbyRoot := tview.NewGrid().
		SetRows(0, 1).
		SetColumns(0).
		AddItem(lobbyTable, 0, 0, 1, 1, 0, 0, true).
		AddItem(ui.lobbyInput, 1, 0, 1, 1, 0, 0, false)
	tableRoot := tview.NewGrid().
		SetRows(0, 1).
		SetColumns(0).
		AddItem(board, 0, 0, 1, 1, 0, 0, false).
		AddItem(ui.tableInput, 1, 0, 1, 1, 0, 0, true)
	ui.pages.AddPage(phaseLogin, centerBox(loginForm, 76, 13), true, true)
	ui.pages.AddPage(phaseLobby, lobbyRoot, true, false)
	ui.pages.AddPage(phaseTable, tableRoot, true, false)
	app.SetRoot(ui.pages, true)
	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		view := state.Snapshot()
		switch event.Rune() {
		case 'q':
			if !ui.anyInputFocused() {
				app.Stop()
				return nil
			}
		case 'm':
			if view.Phase == phaseLobby && !ui.anyInputFocused() {
				_ = handler.Handle(context.Background(), "match")
				return nil
			}
		case 'n':
			if view.Phase == phaseLobby && !ui.anyInputFocused() {
				ui.openCreateForm()
				return nil
			}
		case '1', '2', '3', '4', '5', '6', '7', '8', '9':
			if view.Phase == phaseTable && !ui.anyInputFocused() {
				handler.DiscardIndex(context.Background(), int(event.Rune()-'1'))
				return nil
			}
		case '0':
			if view.Phase == phaseTable && !ui.anyInputFocused() {
				handler.DiscardIndex(context.Background(), 9)
				return nil
			}
		}
		return event
	})
	return ui
}

func (ui *UI) configureLoginForm() {
	wsURL, name := ui.client.Config()
	ui.loginForm.
		AddInputField("昵称", name, 32, nil, nil).
		AddInputField("服务器", wsURL, 48, nil, nil).
		AddButton("进入大厅", func() {
			name := ui.loginForm.GetFormItemByLabel("昵称").(*tview.InputField).GetText()
			server := ui.loginForm.GetFormItemByLabel("服务器").(*tview.InputField).GetText()
			ui.client.SetConfig(server, name)
			ui.state.Mutate(func(v *RoomView) {
				v.Nickname = name
				v.ServerURL = server
			})
			ui.state.AddLog("正在连接大厅")
			ui.startClient()
		}).
		AddButton("退出", func() { ui.app.Stop() })
	ui.loginForm.SetBorder(true).SetTitle(" lsp-cli 登录 ")
}

func (ui *UI) newCommandInput(label string) *tview.InputField {
	input := tview.NewInputField().SetLabel(label).SetFieldWidth(0)
	input.SetDoneFunc(func(key tcell.Key) {
		if key != tcell.KeyEnter {
			return
		}
		line := input.GetText()
		input.SetText("")
		if !ui.handler.Handle(context.Background(), line) {
			ui.app.Stop()
		}
	})
	return input
}

func (ui *UI) startClient() {
	ui.startMu.Lock()
	defer ui.startMu.Unlock()
	if ui.started {
		return
	}
	ui.started = true
	ctx := ui.runCtx
	if ctx == nil {
		ctx = context.Background()
	}
	go ui.client.Run(ctx)
}

func (ui *UI) openCreateForm() {
	form := tview.NewForm().
		AddInputField("规则", "sichuan_xzdd", 24, nil, nil).
		AddInputField("房间名", "", 32, nil, nil).
		AddCheckbox("私密", false, nil)
	form.AddButton("创建", func() {
		rule := form.GetFormItemByLabel("规则").(*tview.InputField).GetText()
		name := form.GetFormItemByLabel("房间名").(*tview.InputField).GetText()
		private := form.GetFormItemByLabel("私密").(*tview.Checkbox).IsChecked()
		_ = ui.handler.sendCreateRoom(context.Background(), rule, name, private)
		ui.pages.RemovePage("create")
	}).AddButton("取消", func() { ui.pages.RemovePage("create") })
	form.SetBorder(true).SetTitle(" 创建房间 ")
	ui.pages.AddPage("create", centerBox(form, 64, 12), true, true)
}

func (ui *UI) Run(ctx context.Context) error {
	ui.runCtx = ctx
	go ui.refreshLoop(ctx)
	return ui.app.Run()
}

func (ui *UI) Stop() {
	ui.app.Stop()
}

func (ui *UI) refreshLoop(ctx context.Context) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			ui.app.QueueUpdateDraw(func() { ui.app.Stop() })
			return
		case <-ticker.C:
			ui.app.QueueUpdateDraw(func() {
				view := ui.state.Snapshot()
				ui.switchPhase(view.Phase)
				ui.renderLobby(view)
				_, _, width, height := ui.board.GetRect()
				opts := ui.opts
				opts.Width = width
				opts.Height = height
				ui.board.SetText(strings.Join(RenderLines(view, opts), "\n"))
			})
		}
	}
}

func (ui *UI) switchPhase(phase string) {
	if phase == "" {
		phase = phaseLogin
	}
	ui.pages.SwitchToPage(phase)
}

func (ui *UI) renderLobby(view RoomView) {
	ui.lobbyTable.Clear()
	headers := []string{"房间", "规则", "玩家", "阶段", "名称"}
	for col, header := range headers {
		ui.lobbyTable.SetCell(0, col, tview.NewTableCell(header).SetTextColor(tcell.ColorYellow).SetSelectable(false))
	}
	if len(view.RoomList) == 0 {
		ui.lobbyTable.SetCell(1, 0, tview.NewTableCell("暂无公开房间，按 m 自动匹配或按 n 创建房间").SetExpansion(1))
		return
	}
	for row, room := range view.RoomList {
		r := row + 1
		ui.lobbyTable.SetCell(r, 0, tview.NewTableCell(room.GetRoomId()))
		ui.lobbyTable.SetCell(r, 1, tview.NewTableCell(room.GetRuleId()))
		ui.lobbyTable.SetCell(r, 2, tview.NewTableCell(fmt.Sprintf("%d/%d", room.GetSeatCount(), room.GetMaxSeats())))
		ui.lobbyTable.SetCell(r, 3, tview.NewTableCell(room.GetStage()))
		ui.lobbyTable.SetCell(r, 4, tview.NewTableCell(room.GetDisplayName()))
	}
	ui.lobbyTable.SetSelectedFunc(func(row, _ int) {
		if row <= 0 || row > len(view.RoomList) {
			return
		}
		_ = ui.handler.Handle(context.Background(), "join "+view.RoomList[row-1].GetRoomId())
	})
}

func (ui *UI) anyInputFocused() bool {
	return ui.lobbyInput.HasFocus() || ui.tableInput.HasFocus() || ui.loginForm.HasFocus()
}

func centerBox(p tview.Primitive, width, height int) tview.Primitive {
	return tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(p, width, 0, true).
			AddItem(nil, 0, 1, false), height, 0, true).
		AddItem(nil, 0, 1, false)
}
