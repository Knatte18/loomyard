// rawworktree_test.go pins isRawWarpWorktree's anchor-awareness: the _lyx management-marker probe
// must look at the worktree's ANCHORED directory, the only place fabric ever wires the junction —
// a root-joined probe misclassified every subpath-anchored managed worktree as raw.
// Untagged deliberately: the probe is pure filesystem inspection, so this regression guard runs in
// Tier 1 and stays visible to a plain untagged test run.

package fabricengine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

func TestIsRawWarpWorktree_ProbesAnchoredDirectory(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		anchorRel string
		lyxDirAt  string // relative dir that holds _lyx, "" for none
		want      bool
	}{
		{"SubpathAnchoredManaged", "backend", "backend", false},
		{"RootAnchoredManaged", ".", ".", false},
		{"SubpathAnchoredRaw", "backend", "", true},
		{"RootLyxDoesNotManageSubpathAnchor", "backend", ".", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			hub := t.TempDir()
			worktree := filepath.Join(hub, "wt")
			if err := os.MkdirAll(filepath.Join(worktree, "backend"), 0o755); err != nil {
				t.Fatalf("mkdir worktree: %v", err)
			}
			if tt.lyxDirAt != "" {
				if err := os.MkdirAll(filepath.Join(worktree, tt.lyxDirAt, lyxdirs.LyxDirName), 0o755); err != nil {
					t.Fatalf("mkdir _lyx: %v", err)
				}
			}

			l := &lyxcwd.Location{HubPath: hub, WorktreeName: "wt", AnchorRel: tt.anchorRel}
			if got := isRawWarpWorktree(l); got != tt.want {
				t.Errorf("isRawWarpWorktree(anchor %q, _lyx at %q) = %v; want %v", tt.anchorRel, tt.lyxDirAt, got, tt.want)
			}
		})
	}
}
