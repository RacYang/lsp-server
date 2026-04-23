package router

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSanitizeRoomID(t *testing.T) {
	t.Parallel()
	require.Equal(t, "r1", SanitizeRoomID("  r1  "))
}
