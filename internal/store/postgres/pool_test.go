package postgres_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"racoo.cn/lsp/internal/store/postgres"
)

func TestOpenPoolEmptyDSN(t *testing.T) {
	t.Parallel()
	_, err := postgres.OpenPool(context.Background(), "")
	require.Error(t, err)
}
