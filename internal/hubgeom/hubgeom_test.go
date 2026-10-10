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
	"github.com/Knatte18/loomyard/internal/hubreconcile"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/reedengine"
)

// TestReedGeometry pins every ReedGeometry field against a fixture whose hub, worktree root and anchor path are distinct directories.
// The prime is told no slug and a task worktree its own name as the slug.
//
//testtiming:keep pins the field-by-field geometry of a prime and of an anchored task worktree, which the integration tests covering its blocks only see through a real hub
func TestReedGeometry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		anchorRel string
		isPrime   bool
		wantSlug  string
	}{
		{"subpath-anchored fixture", filepath.Join("sub", "dir"), false, "some-worktree"},
		{"unanchored fixture", ".", false, "some-worktree"},
		{"prime fixture", ".", true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

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

			got := reedGeometry(l, tt.isPrime)

			if got.SpawnOrder == nil {
				t.Error("ReedGeometry(l).SpawnOrder = nil, want the hub's spawn-order teller")
			}

			if got.NameSlug != tt.wantSlug {
				t.Errorf("ReedGeometry(l).NameSlug = %q; want %q", got.NameSlug, tt.wantSlug)
			}

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
			if want := filepath.Join(fabricengine.HubScratchDir(hub), reedengine.DiscoverSignalFileName); got.DiscoverSignalPath != want {
				t.Errorf("ReedGeometry(l).DiscoverSignalPath = %q; want %q", got.DiscoverSignalPath, want)
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

// TestBurlerGeometry pins BurlerGeometry's roots against a subpath-anchored fixture, whose hub, worktree root and anchor path are three distinct directories.
//
//testtiming:keep pins that WorktreeRoot is the anchor path while RepoRoot stays the worktree path, which the origin-record test covering its blocks never reads
func TestBurlerGeometry(t *testing.T) {
	t.Parallel()

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
		// The subpath-anchored fixture must catch a later "simplification" that repoints BurlerGeometry's WorktreeRoot fill at l.WorktreePath(): the two only coincide when AnchorRel is ".", which this fixture deliberately is not.
		t.Errorf("BurlerGeometry(l).WorktreeRoot = %q; want != WorktreeRoot %q", got.WorktreeRoot, worktreeRoot)
	}
}

// TestBurlerGeometry_ParentNameFromOriginRecord pins ParentName to what the worktree's origin record names, and to empty when the record names no parent worktree.
//
//testtiming:keep pins ParentName from a hand-written origin record, which the roots test covering its blocks never builds
func TestBurlerGeometry_ParentNameFromOriginRecord(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

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
			t.Parallel()

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

// TestReconcileGeometry verifies a hub with a board-level lyx dir yields exactly the board dir and worktree root, and a layout without one yields the zero value and false.
func TestReconcileGeometry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		boardLyxDir  bool
		wantGeometry bool
	}{
		{"hub with a board lyx dir", true, true},
		{"layout without a board lyx dir", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			hub := filepath.Join(t.TempDir(), "some-hub-LYXHUB")
			l := &lyxcwd.Location{HubPath: hub, WorktreeName: "some-worktree", AnchorRel: "."}
			mkdir(t, l.WorktreePath())
			if tt.boardLyxDir {
				mkdir(t, filepath.Join(fabricengine.BoardDir(hub), lyxdirs.LyxDirName))
			} else {
				mkdir(t, fabricengine.BoardDir(hub))
			}

			got, ok := ReconcileGeometry(l)

			want := hubreconcile.Geometry{}
			if tt.wantGeometry {
				want = hubreconcile.Geometry{BoardDir: fabricengine.BoardDir(hub), WorktreePath: l.WorktreePath()}
			}
			if ok != tt.wantGeometry || got != want {
				t.Errorf("ReconcileGeometry() = (%+v, %v); want (%+v, %v)", got, ok, want, tt.wantGeometry)
			}
		})
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
