package discovery

import (
	"testing"

	"github.com/stretchr/testify/require"

	"racoo.cn/lsp/internal/cluster/nodeid"
)

func TestNodeMeta_zero(t *testing.T) {
	t.Parallel()
	var m NodeMeta
	require.Empty(t, m.AdvertiseAddr)
}

func TestNodeInfo_fields(t *testing.T) {
	t.Parallel()
	n := NodeInfo{NodeID: "x", Kind: nodeid.KindGate, Meta: NodeMeta{AdvertiseAddr: "127.0.0.1:1"}}
	require.Equal(t, nodeid.KindGate, n.Kind)
}

func TestKindString(t *testing.T) {
	t.Parallel()
	require.Equal(t, "gate", KindString(nodeid.KindGate))
}
