// hubgeom_test.go is the load-bearing guard against this refactor's one silent failure mode: a
// swapped anchor/worktree pair compiles cleanly and passes every test built on a fixture where the
// two happen to coincide. The fixture below deliberately keeps hub, worktree root, and anchor path
// three distinct directories, with RepoName differing from every basename, so a field mix-up inside
// ReedGeometry or BurlerGeometry surfaces instead of passing silently.

package hubgeom

import (
	"os"
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
			hub := filepath.Join(root, "some-hub-LYXHUB")
			worktreeName := "some-worktree"
			worktreeRoot := filepath.Join(hub, worktreeName)
			anchorPath := filepath.Join(worktreeRoot, tt.anchorRel)

			l := &lyxcwd.Location{
				RepoName:     "distinct-repo-name",
				HubPath:      hub,
				WorktreeName: worktreeName,
				AnchorRel:    tt.anchorRel,
			}

			got := reedGeometry(l, false)

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
			if got.WorktreeName != l.WorktreeName {
				t.Errorf("ReedGeometry(l).WorktreeName = %q; want %q", got.WorktreeName, l.WorktreeName)
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
			hub := filepath.Join(root, "some-hub-LYXHUB")
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
			if got.RepoRoot != worktreeRoot {
				t.Errorf("BurlerGeometry(l).RepoRoot = %q; want %q (l.WorktreePath(), not the anchor path)", got.RepoRoot, worktreeRoot)
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

// TestBurlerGeometry_ParentNameFromOriginRecord pins ParentName to what the worktree's origin record names, and to empty when the record names no parent worktree.
func TestBurlerGeometry_ParentNameFromOriginRecord(t *testing.T) {
	l := hubWithOrigin(t, `{"parent_branch":"main","parent_worktree":"gone-task"}`)
	if got, want := BurlerGeometry(l).ParentName, "tst:gone-task:orch"; got != want {
		t.Errorf("BurlerGeometry(l).ParentName = %q; want %q", got, want)
	}

	l = hubWithOrigin(t, `{"parent_branch":"main"}`)
	if got := BurlerGeometry(l).ParentName; got != "" {
		t.Errorf("BurlerGeometry(l).ParentName without a parent worktree = %q; want empty", got)
	}
}

// TestIsPrimeWorktree pins the .git-entry rule ReedGeometry tells the prime from a task worktree by.
// A directory, or a gitdir file pointing straight at a git directory, is the main worktree.
// A gitdir file pointing into worktrees/ is a linked one, and anything else is an error.
func TestIsPrimeWorktree(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T, root string)
		wantPrime bool
		wantErr   bool
	}{
		{"git directory is the prime", func(t *testing.T, root string) {
			mkdir(t, filepath.Join(root, ".git"))
		}, true, false},
		{"gitdir file into worktrees is a task worktree", func(t *testing.T, root string) {
			writeFile(t, filepath.Join(root, ".git"), "gitdir: /hub/prime/.git/worktrees/some-task\n")
		}, false, false},
		{"gitdir file to a separate git directory is the prime", func(t *testing.T, root string) {
			writeFile(t, filepath.Join(root, ".git"), "gitdir: /elsewhere/repo.git\n")
		}, true, false},
		{"missing entry is an error", func(t *testing.T, root string) {}, false, true},
		{"file without gitdir is an error", func(t *testing.T, root string) {
			writeFile(t, filepath.Join(root, ".git"), "not a pointer\n")
		}, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			tt.setup(t, root)

			prime, err := isPrimeWorktree(root)
			if (err != nil) != tt.wantErr {
				t.Fatalf("isPrimeWorktree() error = %v; want error %v", err, tt.wantErr)
			}
			if err == nil && prime != tt.wantPrime {
				t.Errorf("isPrimeWorktree() = %v; want %v", prime, tt.wantPrime)
			}
		})
	}
}

// TestReedGeometry_PrimeAndTaskSlug asserts the prime is told no slug and a task worktree its own name as the slug.
func TestReedGeometry_PrimeAndTaskSlug(t *testing.T) {
	l := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "some-task", AnchorRel: "."}

	if got := reedGeometry(l, true).NameSlug; got != "" {
		t.Errorf("reedGeometry(prime).NameSlug = %q; want empty", got)
	}
	if got := reedGeometry(l, false).NameSlug; got != "some-task" {
		t.Errorf("reedGeometry(task).NameSlug = %q; want %q", got, "some-task")
	}
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v", path, err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v", path, err)
	}
}
