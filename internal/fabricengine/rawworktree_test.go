// rawworktree_test.go pins isRawWarpWorktree's anchor-awareness: the _lym management-marker probe
// must look at the worktree's anchored directory, the only place fabric wires the junction.
// A root-joined probe misclassifies every subpath-anchored managed worktree as raw.
// The test runs in tier0 because the probe is pure filesystem inspection.

package fabricengine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestIsRawWarpWorktree_ProbesAnchoredDirectory(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name      string
		anchorRel string
		lymDirAt  string // relative dir that holds _lym, "" for none
		want      bool
	}{
		{"SubpathAnchoredManaged", "backend", "backend", false},
		{"RootAnchoredManaged", ".", ".", false},
		{"SubpathAnchoredRaw", "backend", "", true},
		{"RootLymDoesNotManageSubpathAnchor", "backend", ".", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			hub := t.TempDir()
			worktree := filepath.Join(hub, "wt")
			if err := os.MkdirAll(filepath.Join(worktree, "backend"), 0o755); err != nil {
				t.Fatalf("mkdir worktree: %v", err)
			}
			if tt.lymDirAt != "" {
				if err := os.MkdirAll(filepath.Join(worktree, tt.lymDirAt, lymdirs.LymDirName), 0o755); err != nil {
					t.Fatalf("mkdir _lym: %v", err)
				}
			}

			l := &lymcwd.Location{HubPath: hub, WorktreeName: "wt", AnchorRel: tt.anchorRel}
			if got := isRawWarpWorktree(l); got != tt.want {
				t.Errorf("isRawWarpWorktree(anchor %q, _lym at %q) = %v; want %v", tt.anchorRel, tt.lymDirAt, got, tt.want)
			}
		})
	}
}
