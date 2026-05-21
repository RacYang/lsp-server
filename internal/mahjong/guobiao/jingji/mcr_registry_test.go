package jingji

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMCRFanRegistryDefinesEightyOneFans(t *testing.T) {
	t.Parallel()

	registry := mcrFanRegistry()
	require.Len(t, registry, 81)
	seen := map[string]struct{}{}
	for _, def := range registry {
		require.NotEmpty(t, def.ID)
		require.NotEmpty(t, def.Label)
		require.Positive(t, def.Points)
		if _, ok := seen[def.ID]; ok {
			t.Fatalf("duplicate MCR fan id %q", def.ID)
		}
		seen[def.ID] = struct{}{}
	}
}
