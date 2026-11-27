// fabric_test.go holds the unit tests for the Fabric handle and its sync options.
// The tests spawn no git: Open only stat-checks paths, over a hand-built *lymcwd.Location.
// The missing-path contract is tested through Open.
// Open checks warp first and returns *ErrMissingPath naming the absent side.
// TestEnvSyncOptions covers the environment mapping and TestRequireWarpWorktree the warp-worktree guard.

package fabricengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/weftname"
)

// TestOpen covers Open over a hand-built Location.
// A missing warp worktree errors with *ErrMissingPath naming the warp path.
// The weft sibling exists in that case, so only the warp side can fail, which proves Open checks warp first.
// A missing weft sibling errors naming the weft path when the warp worktree is present.
// When both exist, Open returns a non-nil handle.
//
//testtiming:keep Open returning *ErrMissingPath naming the warp path first, then the weft path, and a non-nil handle when both exist; coverage of its blocks by other tests does not show an assertion of this
func TestOpen(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name      string
		mkWarp    bool
		mkWeft    bool
		wantError bool
	}{
		{"missing_warp_path", false, true, true},
		{"missing_weft_path", true, false, true},
		{"happy_path", true, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l := &lymcwd.Location{HubPath: t.TempDir(), WorktreeName: "warp", AnchorRel: "."}
			if tt.mkWarp {
				if err := os.Mkdir(l.WorktreePath(), 0755); err != nil {
					t.Fatalf("mkdir warp worktree: %v", err)
				}
			}
			if tt.mkWeft {
				if err := os.Mkdir(fabricengine.RecordsWorktree(l), 0755); err != nil {
					t.Fatalf("mkdir weft sibling: %v", err)
				}
			}
			wantMissing := l.WorktreePath()
			if tt.mkWarp {
				wantMissing = fabricengine.RecordsWorktree(l)
			}

			f, err := fabricengine.Open(l)
			if !tt.wantError {
				if err != nil {
					t.Fatalf("Open() error = %v", err)
				}
				if f == nil {
					t.Fatal("Open() = nil; want non-nil handle")
				}
				return
			}
			if err == nil {
				t.Fatalf("Open() error = nil; want error naming %q", wantMissing)
			}
			var missingPath *fabricengine.ErrMissingPath
			if !errors.As(err, &missingPath) {
				t.Fatalf("Open() error = %v; want *ErrMissingPath", err)
			}
			if missingPath.Path != wantMissing {
				t.Errorf("Open() error path = %q; want %q", missingPath.Path, wantMissing)
			}
		})
	}
}

// TestEnvSyncOptions covers the FABRIC_SKIP_GIT / FABRIC_SKIP_PUSH mapping: unset, "1", and other values.
func TestEnvSyncOptions(t *testing.T) {
	tiergate.Require(t, 0)
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
				t.Setenv("FABRIC_SKIP_GIT", tt.skipGitVal)
			} else {
				t.Setenv("FABRIC_SKIP_GIT", "")
			}
			if tt.setSkipPush {
				t.Setenv("FABRIC_SKIP_PUSH", tt.skipPushVal)
			} else {
				t.Setenv("FABRIC_SKIP_PUSH", "")
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

// TestRequireWarpWorktree covers the guard that refuses a cwd inside one of fabric's own checkouts.
// Such a cwd would otherwise drive a verb against invented geometry.
// lymcwd resolves such a cwd without error, because it knows neither the weft sibling nor the board checkout.
// The refusal therefore has to happen in fabric.
// The guard is pure name math over a hand-built Location, so the test spawns no git.
func TestRequireWarpWorktree(t *testing.T) {
	tiergate.Require(t, 0)
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
		{name: "the hub board checkout", worktreeName: fabriclayout.BoardDirName, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := &lymcwd.Location{HubPath: filepath.Join("/hub", "myrepo-LYMHUB"), WorktreeName: tt.worktreeName, AnchorRel: "."}
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
