// hubgeom_test.go is the load-bearing guard against this refactor's one silent failure mode: a
// swapped anchor/worktree pair compiles cleanly and passes every test built on a fixture where the
// two happen to coincide. The fixture below deliberately keeps hub, worktree root, and anchor path
// three distinct directories, with RepoName differing from every basename, so a field mix-up inside
// ReedGeometry or BurlerGeometry surfaces instead of passing silently.

package hubgeom

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/reedengine"
)

func TestReedGeometry(t *testing.T) {
	tests := []struct {
		name      string
		anchorRel string
	}{
		{"subpath-anchored fixture", filepath.Join("sub", "dir")},
		{"unanchored fixture", "."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			hub := filepath.Join(root, "some-hub-HUB")
			worktreeName := "some-worktree"
			worktreeRoot := filepath.Join(hub, worktreeName)
			anchorPath := filepath.Join(worktreeRoot, tt.anchorRel)

			l := &lyxcwd.Location{
				RepoName:     "distinct-repo-name",
				HubPath:      hub,
				WorktreeName: worktreeName,
				AnchorRel:    tt.anchorRel,
			}

			got := ReedGeometry(l)

			if want := reedengine.ServerName(hub); got.SocketKey != want {
				t.Errorf("ReedGeometry(l).SocketKey = %q; want %q (ServerName(hub))", got.SocketKey, want)
			}
			if want := reedengine.SessionName(worktreeRoot); got.SessionName != want {
				t.Errorf("ReedGeometry(l).SessionName = %q; want %q (SessionName(worktreeRoot))", got.SessionName, want)
			}
			if got.AnchorPath != anchorPath {
				t.Errorf("ReedGeometry(l).AnchorPath = %q; want %q", got.AnchorPath, anchorPath)
			}
			if got.PaneCwd != l.AnchorPath() {
				t.Errorf("ReedGeometry(l).PaneCwd = %q; want %q (l.AnchorPath())", got.PaneCwd, l.AnchorPath())
			}
			if tt.anchorRel != "." && got.PaneCwd == worktreeRoot {
				// The subpath-anchored row must catch a later "simplification"
				// that repoints the spawn sites at WorktreeRoot: the two only
				// coincide when AnchorRel is ".", which this row deliberately is
				// not.
				t.Errorf("ReedGeometry(l).PaneCwd = %q; want != WorktreeRoot %q", got.PaneCwd, worktreeRoot)
			}
			if got.WorktreeRoot != worktreeRoot {
				t.Errorf("ReedGeometry(l).WorktreeRoot = %q; want %q", got.WorktreeRoot, worktreeRoot)
			}
			if want := fabricengine.HubLogsDir(hub); got.LogsDir != want {
				t.Errorf("ReedGeometry(l).LogsDir = %q; want %q", got.LogsDir, want)
			}
			if got.RepoName != l.RepoName {
				t.Errorf("ReedGeometry(l).RepoName = %q; want %q", got.RepoName, l.RepoName)
			}
			if got.HubPath != hub {
				t.Errorf("ReedGeometry(l).HubPath = %q; want %q", got.HubPath, hub)
			}
		})
	}
}

func TestBurlerGeometry(t *testing.T) {
	tests := []struct {
		name string
	}{
		{"subpath-anchored fixture"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			hub := filepath.Join(root, "some-hub-HUB")
			worktreeName := "some-worktree"
			worktreeRoot := filepath.Join(hub, worktreeName)
			anchorRel := filepath.Join("sub", "dir")
			anchorPath := filepath.Join(worktreeRoot, anchorRel)

			l := &lyxcwd.Location{
				RepoName:     "distinct-repo-name",
				HubPath:      hub,
				WorktreeName: worktreeName,
				AnchorRel:    anchorRel,
			}

			var got burlerengine.Geometry = BurlerGeometry(l)

			if got.WorktreeRoot != anchorPath {
				t.Errorf("BurlerGeometry(l).WorktreeRoot = %q; want %q (anchorPath)", got.WorktreeRoot, anchorPath)
			}
			if got.AnchorPath != anchorPath {
				t.Errorf("BurlerGeometry(l).AnchorPath = %q; want %q", got.AnchorPath, anchorPath)
			}
			if got.WorktreeRoot == worktreeRoot {
				// The subpath-anchored fixture must catch a later
				// "simplification" that repoints BurlerGeometry's
				// WorktreeRoot fill at l.WorktreePath(): the two only
				// coincide when AnchorRel is ".", which this row
				// deliberately is not.
				t.Errorf("BurlerGeometry(l).WorktreeRoot = %q; want != WorktreeRoot %q", got.WorktreeRoot, worktreeRoot)
			}
		})
	}
}
