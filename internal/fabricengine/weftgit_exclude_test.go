//go:build integration

// weftgit_exclude_test.go proves fabric's own lock artifacts (.weft/ from
// CommitWeft's write lock, .gitrepo-push.lock from PushCoalesced) never
// surface as untracked dirt in the weft worktree: ensureWeftLockDir seeds
// them into the weft repo's info/exclude, so Remove's no-force dirty gate
// (a raw `git status --porcelain`, untracked included) cannot dead-end on
// artifacts a pathspec-scoped `fabric sync` can never clear.
//
// Package fabricengine_test to reuse weftgit_differential_test.go's
// newFabricPair/writeWeftConfig/gitStatusPorcelain helpers; shares the
// TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitrepo"
)

// TestCommitWeft_LockArtifactsExcludedFromStatus commits scoped weft content
// (which creates the .weft lock dir) and drops a push lock file, then asserts
// neither artifact appears in `git status --porcelain` — the exact check
// Remove's no-force weft dirty gate runs.
func TestCommitWeft_LockArtifactsExcludedFromStatus(t *testing.T) {
	f, weftFixture := newFabricPair(t)
	writeWeftConfig(t, weftFixture.WeftPath, "modified for exclude test")

	if _, committed, err := f.CommitWeft([]string{"_lyx"}, fabricengine.DefaultCommitMessage, fabricengine.SyncOptions{}); err != nil {
		t.Fatalf("CommitWeft: %v", err)
	} else if !committed {
		t.Fatal("CommitWeft committed = false; want true")
	}

	// Precondition: the commit really did materialize the lock dir — the
	// artifact this test claims is excluded must actually exist on disk.
	if _, err := os.Stat(filepath.Join(weftFixture.WeftPath, ".weft")); err != nil {
		t.Fatalf(".weft lock dir missing after CommitWeft: %v", err)
	}

	// Materialize the push lock artifact the way an interrupted PushCoalesced
	// would leave it: a plain file at the worktree root.
	pushLock := filepath.Join(weftFixture.WeftPath, gitrepo.PushLockFileName)
	if err := os.WriteFile(pushLock, nil, 0o644); err != nil {
		t.Fatalf("write push lock artifact: %v", err)
	}

	status := gitStatusPorcelain(t, weftFixture.WeftPath)
	if strings.Contains(status, ".weft") {
		t.Errorf("git status --porcelain reports .weft as dirt: %q; want it git-excluded", status)
	}
	if strings.Contains(status, gitrepo.PushLockFileName) {
		t.Errorf("git status --porcelain reports %s as dirt: %q; want it git-excluded", gitrepo.PushLockFileName, status)
	}
}
