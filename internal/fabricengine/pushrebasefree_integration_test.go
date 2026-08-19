//go:build integration

// pushrebasefree_integration_test.go covers PushWarpRebaseFreeAt against a real warp checkout with a
// bare origin: a commit ahead of its upstream pushes successfully and the returned record reports
// the branch push; SkipGit and SkipPush each return an empty result with a nil error and push
// nothing; and no untracked push-lock file is left behind — the residue property this function exists
// to guarantee. Reuses coalesce_integration_test.go's addWarpBareRemote and commitPlain fixture
// helpers and gitsha_integration_test.go's BareBranchSHAForTest/CurrentSHAForTest re-exports.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitrepo"
)

// TestPushWarpRebaseFreeAt_PushesAndRecordsBranchPush covers the successful push path: a warp
// checkout with a commit ahead of its bare origin pushes it, the bare remote advances to the local
// HEAD, and the returned record contains a KindBranchPushed entry.
func TestPushWarpRebaseFreeAt_PushesAndRecordsBranchPush(t *testing.T) {
	fixtures := t.TempDir()

	warpPath := fabricengine.NewPlainWarpRepoForTest(t)
	warpBare := addWarpBareRemote(t, fixtures, warpPath)
	warpSHA := commitPlain(t, warpPath, "warp-file.txt", "warp change")

	res, err := fabricengine.PushWarpRebaseFreeAt(warpPath, fabricengine.SyncOptions{})
	if err != nil {
		t.Fatalf("PushWarpRebaseFreeAt() error = %v; want nil", err)
	}

	if got := fabricengine.BareBranchSHAForTest(t, warpBare, "main"); got != warpSHA {
		t.Errorf("warp bare main = %q; want it advanced to local HEAD %q", got, warpSHA)
	}

	entries := res.Mutated().Entries()
	found := false
	for _, entry := range entries {
		if entry.Kind == fabricengine.KindBranchPushed {
			found = true
		}
	}
	if !found {
		t.Errorf("PushWarpRebaseFreeAt() record = %+v; want a KindBranchPushed entry", entries)
	}
}

// TestPushWarpRebaseFreeAt_SkipGitOrSkipPush_PushesNothing asserts SkipGit and SkipPush each short-
// circuit to an empty result and a nil error, leaving the bare remote unadvanced.
func TestPushWarpRebaseFreeAt_SkipGitOrSkipPush_PushesNothing(t *testing.T) {
	tests := []struct {
		name string
		opts fabricengine.SyncOptions
	}{
		{name: "SkipGit", opts: fabricengine.SyncOptions{SkipGit: true}},
		{name: "SkipPush", opts: fabricengine.SyncOptions{SkipPush: true}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixtures := t.TempDir()

			warpPath := fabricengine.NewPlainWarpRepoForTest(t)
			warpBare := addWarpBareRemote(t, fixtures, warpPath)
			bareHeadBefore := fabricengine.BareBranchSHAForTest(t, warpBare, "main")
			commitPlain(t, warpPath, "warp-file.txt", "warp change never pushed")

			res, err := fabricengine.PushWarpRebaseFreeAt(warpPath, tt.opts)
			if err != nil {
				t.Fatalf("PushWarpRebaseFreeAt() error = %v; want nil", err)
			}
			if res.Mutated().Len() != 0 {
				t.Errorf("PushWarpRebaseFreeAt() record = %+v; want empty", res.Mutated().Entries())
			}

			if got := fabricengine.BareBranchSHAForTest(t, warpBare, "main"); got != bareHeadBefore {
				t.Errorf("warp bare main = %q; want it unadvanced at %q", got, bareHeadBefore)
			}
		})
	}
}

// TestPushWarpRebaseFreeAt_LeavesNoPushLockResidue asserts the residue property PushWarpRebaseFreeAt
// exists to guarantee: after a successful push, no gitrepo.PushLockFileName file is left behind at
// the warp worktree root.
func TestPushWarpRebaseFreeAt_LeavesNoPushLockResidue(t *testing.T) {
	fixtures := t.TempDir()

	warpPath := fabricengine.NewPlainWarpRepoForTest(t)
	addWarpBareRemote(t, fixtures, warpPath)
	commitPlain(t, warpPath, "warp-file.txt", "warp change")

	if _, err := fabricengine.PushWarpRebaseFreeAt(warpPath, fabricengine.SyncOptions{}); err != nil {
		t.Fatalf("PushWarpRebaseFreeAt() error = %v; want nil", err)
	}

	if _, err := os.Stat(filepath.Join(warpPath, gitrepo.PushLockFileName)); !os.IsNotExist(err) {
		t.Errorf("%s exists at warp worktree root after PushWarpRebaseFreeAt(); want it absent (lock-free push)", gitrepo.PushLockFileName)
	}
}
