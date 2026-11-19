// hubsetsgate_test.go covers HubSetsGate, which reads the hub's own gate.yaml alone.

package gateslot

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/testkit/permkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestHubSetsGate(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const key = "harden-live"
	tests := []struct {
		name string
		hub  string
		// worktreeLayer is written as the worktree's local gate layer over the hub file.
		worktreeLayer string
		unreadable    bool
		want          bool
		wantErr       bool
	}{
		{name: "HubFileWithoutTheKey", hub: baseKeys + "gates:\n  card: {tiers: [0], scope: card}\n", want: false},
		{name: "HubFileSettingTheKey", hub: baseKeys + "gates:\n  harden-live: {tiers: [2], scope: diff}\n", want: true},
		{
			name: "WorktreeLayerSettingTheKeyOverAHubFileWithout", hub: baseKeys,
			worktreeLayer: "gates:\n  harden-live: {tiers: [2], scope: diff}\n", want: false,
		},
		{name: "UnreadableHubFile", hub: baseKeys, unreadable: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			board := seedGateConfig(t, tt.hub)
			anchor := t.TempDir()
			if tt.worktreeLayer != "" {
				layer := configengine.LocalFile(anchor, "gate")
				if err := os.MkdirAll(filepath.Dir(layer), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(layer, []byte(tt.worktreeLayer), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if tt.unreadable {
				permkit.Chmod(t, configengine.ConfigFile(board, "gate"), 0)
			}

			got, err := HubSetsGate(configengine.Dirs{Board: board, Anchor: anchor}, key)

			if (err != nil) != tt.wantErr {
				t.Fatalf("HubSetsGate(%q) error = %v; want error %v", key, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("HubSetsGate(%q) = %v; want %v", key, got, tt.want)
			}
		})
	}
}
