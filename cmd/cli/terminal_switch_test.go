package main

import (
	"errors"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/require"
)

func TestTerminalSwitchEnterAndLeaveLifecycle(t *testing.T) {
	sim := tcell.NewSimulationScreen("UTF-8")
	sw := NewTerminalSwitchWithFactory(func() (tcell.Screen, error) { return sim, nil })

	require.False(t, sw.IsFullscreen())

	scr, err := sw.EnterFullscreen()
	require.NoError(t, err)
	require.True(t, sw.IsFullscreen())
	require.Same(t, sim, scr)

	sw.LeaveFullscreen()
	require.False(t, sw.IsFullscreen())
}

func TestTerminalSwitchRejectsDoubleEnter(t *testing.T) {
	sim := tcell.NewSimulationScreen("UTF-8")
	sw := NewTerminalSwitchWithFactory(func() (tcell.Screen, error) { return sim, nil })

	_, err := sw.EnterFullscreen()
	require.NoError(t, err)
	defer sw.LeaveFullscreen()

	_, err = sw.EnterFullscreen()
	require.Error(t, err)
}

func TestTerminalSwitchLeaveIsSafeWithoutEnter(t *testing.T) {
	sw := NewTerminalSwitchWithFactory(func() (tcell.Screen, error) { return tcell.NewSimulationScreen("UTF-8"), nil })

	sw.LeaveFullscreen()
	require.False(t, sw.IsFullscreen())
}

func TestTerminalSwitchPropagatesFactoryError(t *testing.T) {
	want := errors.New("boom")
	sw := NewTerminalSwitchWithFactory(func() (tcell.Screen, error) { return nil, want })

	_, err := sw.EnterFullscreen()
	require.ErrorIs(t, err, want)
	require.False(t, sw.IsFullscreen())
}

func TestTerminalSwitchAllowsReEnterAfterLeave(t *testing.T) {
	sw := NewTerminalSwitchWithFactory(func() (tcell.Screen, error) { return tcell.NewSimulationScreen("UTF-8"), nil })

	_, err := sw.EnterFullscreen()
	require.NoError(t, err)
	sw.LeaveFullscreen()

	_, err = sw.EnterFullscreen()
	require.NoError(t, err)
	sw.LeaveFullscreen()
}
