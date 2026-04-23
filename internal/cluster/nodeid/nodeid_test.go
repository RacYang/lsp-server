package nodeid

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNew_unique(t *testing.T) {
	t.Parallel()
	a := New()
	b := New()
	require.NotEqual(t, a, b)
	require.Len(t, strings.ReplaceAll(a, "-", ""), 32)
}

func TestFormat(t *testing.T) {
	t.Parallel()
	require.Equal(t, "room/abc", Format(KindRoom, "abc"))
}
