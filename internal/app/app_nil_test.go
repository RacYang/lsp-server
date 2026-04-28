package app_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"racoo.cn/lsp/internal/app"
)

func TestNilAppAccessors(t *testing.T) {
	t.Parallel()

	var httpApp *app.App
	require.Nil(t, httpApp.Addr())
	require.Error(t, httpApp.Run(context.Background()))

	var grpcApp *app.GRPCApp
	require.Nil(t, grpcApp.Addr())
	require.Error(t, grpcApp.Run(context.Background()))
}
