package main

import (
	"context"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type UI struct {
	app   *tview.Application
	board *tview.TextView
	input *tview.InputField
	state *AppState
	opts  RenderOptions
}

func NewUI(state *AppState, handler *CommandHandler, opts RenderOptions) *UI {
	app := tview.NewApplication()
	board := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(false).
		SetWrap(false)
	input := tview.NewInputField().
		SetLabel("> ").
		SetFieldWidth(0)
	ui := &UI{app: app, board: board, input: input, state: state, opts: opts}
	input.SetDoneFunc(func(key tcell.Key) {
		if key != tcell.KeyEnter {
			return
		}
		line := input.GetText()
		input.SetText("")
		if !handler.Handle(context.Background(), line) {
			app.Stop()
		}
	})
	root := tview.NewGrid().
		SetRows(0, 1).
		SetColumns(0).
		AddItem(board, 0, 0, 1, 1, 0, 0, false).
		AddItem(input, 1, 0, 1, 1, 0, 0, true)
	app.SetRoot(root, true)
	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Rune() {
		case 'q':
			if !input.HasFocus() {
				app.Stop()
				return nil
			}
		case '1', '2', '3', '4', '5', '6', '7', '8', '9':
			if !input.HasFocus() {
				handler.DiscardIndex(context.Background(), int(event.Rune()-'1'))
				return nil
			}
		case '0':
			if !input.HasFocus() {
				handler.DiscardIndex(context.Background(), 9)
				return nil
			}
		}
		return event
	})
	return ui
}

func (ui *UI) Run(ctx context.Context) error {
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
				_, _, width, height := ui.board.GetRect()
				opts := ui.opts
				opts.Width = width
				opts.Height = height
				ui.board.SetText(strings.Join(RenderLines(ui.state.Snapshot(), opts), "\n"))
			})
		}
	}
}
