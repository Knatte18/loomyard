// friction_test.go — untagged Tier-1 unit tests for LoomDurableDir, LoomFrictionDir, LoomFrictionArchivePrefix and LoomFrictionLock.
// Pure path arithmetic over a hand-built lyxcwd.Location, no live hub, reed, or network involved.

package loomengine

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

// TestLoomFrictionPaths verifies the durable and ephemeral friction paths at a subpath anchor,
// whose AnchorRel differs from "." to prove the accessors follow the anchored subpath, not the bare worktree root:
// LoomDurableDir sits under the durable _lyx tree and LoomDurableDirRel is its anchor-relative suffix,
// LoomFrictionDir sits beneath LoomDurableDir, LoomFrictionArchivePrefix is LoomFrictionDir's own return value with a trailing hyphen
// (asserted against it rather than a second hand-built literal, so the two cannot drift),
// and the reflection lock stays ephemeral: under .lyx/loom, with its parent equal to LoomScratchDir.
func TestLoomFrictionPaths(t *testing.T) {
	t.Parallel()
	l := &lyxcwd.Location{
		HubPath:      filepath.Join("home", "user", "repo-LYXHUB"),
		WorktreeName: "repo",
		AnchorRel:    filepath.Join("sub", "dir"),
	}

	wantDurable := filepath.Join(l.AnchorPath(), lyxdirs.LyxDirName, "loom")
	if got := LoomDurableDir(l); got != wantDurable {
		t.Errorf("LoomDurableDir() = %q; want %q", got, wantDurable)
	}
	if got := LoomDurableDirRel(); got != filepath.Join(lyxdirs.LyxDirName, "loom") {
		t.Errorf("LoomDurableDirRel() = %q; want %q", got, filepath.Join(lyxdirs.LyxDirName, "loom"))
	}
	if got := strings.TrimPrefix(LoomDurableDir(l), l.AnchorPath()+string(filepath.Separator)); got != LoomDurableDirRel() {
		t.Errorf("LoomDurableDir() suffix = %q; want LoomDurableDirRel() = %q", got, LoomDurableDirRel())
	}

	wantFriction := filepath.Join(wantDurable, "friction")
	if got := LoomFrictionDir(l); got != wantFriction {
		t.Errorf("LoomFrictionDir() = %q; want %q", got, wantFriction)
	}
	if got := LoomFrictionDir(l); filepath.Dir(got) != LoomDurableDir(l) {
		t.Errorf("LoomFrictionDir() = %q; want its parent to equal LoomDurableDir() = %q", got, LoomDurableDir(l))
	}

	if got, want := LoomFrictionArchivePrefix(l), LoomFrictionDir(l)+"-"; got != want {
		t.Errorf("LoomFrictionArchivePrefix() = %q; want %q", got, want)
	}

	wantLock := filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, "loom", "friction.lock")
	if got := LoomFrictionLock(l); got != wantLock {
		t.Errorf("LoomFrictionLock() = %q; want %q", got, wantLock)
	}
	if got := LoomFrictionLock(l); filepath.Dir(got) != LoomScratchDir(l) {
		t.Errorf("LoomFrictionLock() = %q; want its parent to equal LoomScratchDir() = %q", got, LoomScratchDir(l))
	}
}
