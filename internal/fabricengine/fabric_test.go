// fabric_test.go — unit tests for the Fabric handle, sync options, and ScopedPathspec.
// No git spawn: newPaired only stat-checks paths, and Open needs nothing beyond a hand-built
// *lyxcwd.Location for that same stat-only contract to hold.
// The missing-path contract (warp checked first, *ErrMissingPath naming the absent side) is
// restated here through Open(l), the constructor the contract now belongs to, using a hand-built
// Location rather than a real git fixture — the fast Tier-1 home for this contract.
// open_integration_test.go pins the identical contract end-to-end against a real hub built by the
// hubforge package (TestOpen_MissingWarpWorktree / TestOpen_MissingSiblingWorktree); that is the
// Tier-2 home, not a duplicate of these — these prove the pure stat-check logic without a git spawn.
//
// TestNew_HappyPath below is this package's only remaining NewPairedFromPathsForTest consumer: an
// untagged unit test of the newPaired constructor itself, spawning no git and having nothing to do
// with hub fixtures, so it stays untagged and must never gain a hubforge import — the Test Tier
// Purity Invariant bans an untagged test from calling the hubforge package's hub constructor.

package fabricengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/weftname"
)

// TestOpen_MissingWarpPath asserts that Open errors on a missing warp (warp) worktree, naming the
// warp path — the warp side is checked first, ahead of the weft sibling.
func TestOpen_MissingWarpPath(t *testing.T) {
	hub := t.TempDir()
	l := &lyxcwd.Location{HubPath: hub, WorktreeName: "warp", AnchorRel: "."}

	// The weft sibling exists, so a failure here can only be the warp side —
	// proving Open checks warp first.
	if err := os.Mkdir(fabricengine.WeftWorktree(l), 0755); err != nil {
		t.Fatalf("mkdir weft sibling: %v", err)
	}

	_, err := fabricengine.Open(l)
	if err == nil {
		t.Fatalf("Open() error = nil; want error naming %q", l.WorktreePath())
	}
	var missingPath *fabricengine.ErrMissingPath
	if !errors.As(err, &missingPath) {
		t.Fatalf("Open() error = %v; want *ErrMissingPath", err)
	}
	if missingPath.Path != l.WorktreePath() {
		t.Errorf("Open() error path = %q; want %q", missingPath.Path, l.WorktreePath())
	}
}

// TestOpen_MissingWeftPath asserts that Open errors on a missing weft sibling, naming the weft
// path, when the warp worktree is present.
func TestOpen_MissingWeftPath(t *testing.T) {
	hub := t.TempDir()
	l := &lyxcwd.Location{HubPath: hub, WorktreeName: "warp", AnchorRel: "."}

	if err := os.Mkdir(l.WorktreePath(), 0755); err != nil {
		t.Fatalf("mkdir warp worktree: %v", err)
	}

	_, err := fabricengine.Open(l)
	if err == nil {
		t.Fatalf("Open() error = nil; want error naming %q", fabricengine.WeftWorktree(l))
	}
	var missingPath *fabricengine.ErrMissingPath
	if !errors.As(err, &missingPath) {
		t.Fatalf("Open() error = %v; want *ErrMissingPath", err)
	}
	if missingPath.Path != fabricengine.WeftWorktree(l) {
		t.Errorf("Open() error path = %q; want %q", missingPath.Path, fabricengine.WeftWorktree(l))
	}
}

// TestNew_HappyPath asserts that newPaired yields a non-nil warp and weft when both paths exist as
// directories.
func TestNew_HappyPath(t *testing.T) {
	tmp := t.TempDir()
	warpPath := filepath.Join(tmp, "warp")
	weftPath := filepath.Join(tmp, "weft")
	if err := os.Mkdir(warpPath, 0755); err != nil {
		t.Fatalf("mkdir warp: %v", err)
	}
	if err := os.Mkdir(weftPath, 0755); err != nil {
		t.Fatalf("mkdir weft: %v", err)
	}

	f, err := fabricengine.NewPairedFromPathsForTest(warpPath, weftPath)
	if err != nil {
		t.Fatalf("NewPairedFromPathsForTest() error = %v", err)
	}
	if fabricengine.WarpForTest(f) == nil {
		t.Error("NewPairedFromPathsForTest().warp = nil; want non-nil")
	}
	if fabricengine.WeftForTest(f) == nil {
		t.Error("NewPairedFromPathsForTest().weft = nil; want non-nil")
	}
}

// TestEnvSyncOptions covers the WEFT_SKIP_GIT / WEFT_SKIP_PUSH mapping: unset, "1", and other
// values.
func TestEnvSyncOptions(t *testing.T) {
	tests := []struct {
		name         string
		skipGitVal   string
		setSkipGit   bool
		skipPushVal  string
		setSkipPush  bool
		wantSkipGit  bool
		wantSkipPush bool
	}{
		{name: "unset", wantSkipGit: false, wantSkipPush: false},
		{name: "both_one", setSkipGit: true, skipGitVal: "1", setSkipPush: true, skipPushVal: "1", wantSkipGit: true, wantSkipPush: true},
		{name: "other_values", setSkipGit: true, skipGitVal: "true", setSkipPush: true, skipPushVal: "yes", wantSkipGit: false, wantSkipPush: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setSkipGit {
				t.Setenv("WEFT_SKIP_GIT", tt.skipGitVal)
			} else {
				t.Setenv("WEFT_SKIP_GIT", "")
			}
			if tt.setSkipPush {
				t.Setenv("WEFT_SKIP_PUSH", tt.skipPushVal)
			} else {
				t.Setenv("WEFT_SKIP_PUSH", "")
			}

			got := fabricengine.EnvSyncOptions()
			if got.SkipGit != tt.wantSkipGit {
				t.Errorf("EnvSyncOptions().SkipGit = %v; want %v", got.SkipGit, tt.wantSkipGit)
			}
			if got.SkipPush != tt.wantSkipPush {
				t.Errorf("EnvSyncOptions().SkipPush = %v; want %v", got.SkipPush, tt.wantSkipPush)
			}
		})
	}
}

// TestScopedPathspec covers ScopedPathspec's cases: root relPath (no-op join) and a nested relPath
// (prefixed join).
func TestScopedPathspec(t *testing.T) {
	tests := []struct {
		name    string
		relPath string
		dirs    []string
		want    []string
	}{
		{"root", ".", []string{"_lyx"}, []string{"_lyx"}},
		{"nested", "sub", []string{"_lyx"}, []string{filepath.Join("sub", "_lyx")}},
		{"nested_multiple_dirs", "sub", []string{"_lyx", "_extra"}, []string{filepath.Join("sub", "_lyx"), filepath.Join("sub", "_extra")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fabricengine.ScopedPathspec(tt.relPath, tt.dirs)
			if len(got) != len(tt.want) {
				t.Fatalf("ScopedPathspec(%q, %v) = %v; want %v", tt.relPath, tt.dirs, got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("ScopedPathspec(%q, %v)[%d] = %q; want %q", tt.relPath, tt.dirs, i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestRequireWarpWorktree covers the guard that keeps a cwd inside one of fabric's own checkouts
// from driving a verb against invented geometry. lyxcwd resolves such a cwd cleanly — it has no
// notion of a weft sibling or the board checkout — so the refusal has to happen here.
// No git spawn: the guard is pure name math over a hand-built Location.
func TestRequireWarpWorktree(t *testing.T) {
	tests := []struct {
		name         string
		worktreeName string
		wantErr      bool
	}{
		{name: "ordinary warp prime", worktreeName: "myrepo"},
		{name: "ordinary task worktree", worktreeName: "wt-feature"},
		{name: "a slug merely containing the suffix", worktreeName: "weft-tools"},
		{name: "weft sibling of the prime", worktreeName: "myrepo" + weftname.Suffix, wantErr: true},
		{name: "weft sibling of a task pair", worktreeName: "wt-feature" + weftname.Suffix, wantErr: true},
		{name: "the hub board checkout", worktreeName: fabricengine.BoardDirName, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := &lyxcwd.Location{HubPath: filepath.Join("/hub", "myrepo-HUB"), WorktreeName: tt.worktreeName, AnchorRel: "."}
			err := fabricengine.RequireWarpWorktree(l)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("RequireWarpWorktree(%q) = nil; want a refusal", tt.worktreeName)
				}
				if !errors.Is(err, fabricengine.ErrNotAWarpWorktree) {
					t.Errorf("RequireWarpWorktree(%q) error = %v; want wrapped ErrNotAWarpWorktree", tt.worktreeName, err)
				}
				return
			}
			if err != nil {
				t.Errorf("RequireWarpWorktree(%q) = %v; want nil", tt.worktreeName, err)
			}
		})
	}
}
