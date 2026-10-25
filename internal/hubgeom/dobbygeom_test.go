//go:build tier0

// dobbygeom_test.go pins the paths DobbyGeometry tells dobby for the prime and for a beam, over a temp layout with no git.

package hubgeom

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// TestDobbyGeometry asserts the lock, state and marker paths lie under the named worktree's `.lyx/beam/` directory, the slug is empty for the prime, the batten runs directory lies under the prime, and a worktree with no directory is an error naming it.
func TestDobbyGeometry(t *testing.T) {
	t.Parallel()

	hub := filepath.Join(t.TempDir(), "some-hub-LYXHUB")
	prime := &lyxcwd.Location{RepoName: "some-repo", HubPath: hub, WorktreeName: "some-repo", AnchorRel: "sub"}
	for _, name := range []string{"some-repo", "a-beam"} {
		if err := os.MkdirAll(filepath.Join(hub, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		worktree string
		wantSlug string
	}{
		{"some-repo", ""},
		{"a-beam", "a-beam"},
	}
	for _, tt := range tests {
		t.Run(tt.worktree, func(t *testing.T) {
			t.Parallel()

			got, err := DobbyGeometry(prime, tt.worktree)
			if err != nil {
				t.Fatalf("DobbyGeometry(%q) = %v; want nil", tt.worktree, err)
			}

			root := filepath.Join(hub, tt.worktree)
			beamDir := filepath.Join(root, "sub", lyxdirs.DotLyxDirName, lyxdirs.BeamDirName)
			if got.HubRoot != hub || got.BoardDir != fabricengine.BoardDir(hub) || got.PrimeAnchor != prime.AnchorPath() {
				t.Errorf("DobbyGeometry(%q) hub paths = %q, %q, %q; want %q, %q, %q", tt.worktree, got.HubRoot, got.BoardDir, got.PrimeAnchor, hub, fabricengine.BoardDir(hub), prime.AnchorPath())
			}
			if got.WorktreeRoot != root || got.WorktreeName != tt.worktree || got.Slug != tt.wantSlug {
				t.Errorf("DobbyGeometry(%q) worktree = %q, %q, slug %q; want %q, %q, slug %q", tt.worktree, got.WorktreeRoot, got.WorktreeName, got.Slug, root, tt.worktree, tt.wantSlug)
			}
			for name, path := range map[string]string{"LockPath": got.LockPath, "StatePath": got.StatePath, "MarkerDir": got.MarkerDir} {
				if filepath.Dir(path) != beamDir {
					t.Errorf("DobbyGeometry(%q).%s = %q; want it directly under %q", tt.worktree, name, path, beamDir)
				}
			}
			if want := shedrun.RunsRoot(prime); got.PrimeRunsDir != want {
				t.Errorf("DobbyGeometry(%q).PrimeRunsDir = %q; want %q, under the prime", tt.worktree, got.PrimeRunsDir, want)
			}
			if got.LyxExecutable == "" || !filepath.IsAbs(got.LyxExecutable) {
				t.Errorf("DobbyGeometry(%q).LyxExecutable = %q; want an absolute path", tt.worktree, got.LyxExecutable)
			}
		})
	}

	t.Run("a worktree with no directory is an error naming it", func(t *testing.T) {
		t.Parallel()

		if _, err := DobbyGeometry(prime, "no-such-beam"); err == nil || !strings.Contains(err.Error(), `"no-such-beam"`) {
			t.Errorf("DobbyGeometry(no-such-beam) = %v; want an error naming the worktree", err)
		}
	})
}
