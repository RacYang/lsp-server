package cluster

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewNodeID_unique(t *testing.T) {
	t.Parallel()
	a := NewNodeID()
	b := NewNodeID()
	require.NotEqual(t, a, b)
	require.Len(t, strings.ReplaceAll(a, "-", ""), 32)
}

func TestFormatNodeID(t *testing.T) {
	t.Parallel()
	require.Equal(t, "room/abc", FormatNodeID(KindRoom, "abc"))
}

func TestParseEndpoints(t *testing.T) {
	t.Parallel()
	got := ParseEndpoints(" http://etcd-0:2379, ,http://etcd-1:2379 ")
	require.Equal(t, []string{"http://etcd-0:2379", "http://etcd-1:2379"}, got)
}
