// friction_test.go — untagged Tier-1 unit tests for LoomDurableDir, LoomFrictionDir, LoomFrictionArchivePrefix and LoomFrictionLock.
// Mirrors review_test.go's TestLoomReviewsDir shape: pure path arithmetic over a hand-built
// lyxcwd.Location, no live hub, reed, or network involved.

package loomengine

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

// subpathLocation returns a Location whose AnchorRel differs from "." to prove the accessors follow the anchored subpath, not the bare worktree root.
func subpathLocation() *lyxcwd.Location {
	return &lyxcwd.Location{
		HubPath:      filepath.Join("home", "user", "repo-LYXHUB"),
		WorktreeName: "repo",
		AnchorRel:    filepath.Join("sub", "dir"),
	}
}

// TestLoomDurableDir verifies LoomDurableDir sits under the durable _lyx tree at the anchor and that LoomDurableDirRel is its anchor-relative suffix.
func TestLoomDurableDir(t *testing.T) {
	l := subpathLocation()

	want := filepath.Join(l.AnchorPath(), lyxdirs.LyxDirName, "loom")
	if got := LoomDurableDir(l); got != want {
		t.Errorf("LoomDurableDir() = %q; want %q", got, want)
	}
	if got := LoomDurableDirRel(); got != filepath.Join(lyxdirs.LyxDirName, "loom") {
		t.Errorf("LoomDurableDirRel() = %q; want %q", got, filepath.Join(lyxdirs.LyxDirName, "loom"))
	}
	if got := strings.TrimPrefix(LoomDurableDir(l), l.AnchorPath()+string(filepath.Separator)); got != LoomDurableDirRel() {
		t.Errorf("LoomDurableDir() suffix = %q; want LoomDurableDirRel() = %q", got, LoomDurableDirRel())
	}
}

// TestLoomFrictionDir verifies LoomFrictionDir sits under the durable _lyx/loom tree, with its parent equal to LoomDurableDir.
func TestLoomFrictionDir(t *testing.T) {
	l := subpathLocation()

	want := filepath.Join(l.AnchorPath(), lyxdirs.LyxDirName, "loom", "friction")
	if got := LoomFrictionDir(l); got != want {
		t.Errorf("LoomFrictionDir() = %q; want %q", got, want)
	}
	if got := LoomFrictionDir(l); filepath.Dir(got) != LoomDurableDir(l) {
		t.Errorf("LoomFrictionDir() = %q; want its parent to equal LoomDurableDir() = %q", got, LoomDurableDir(l))
	}
}

// TestLoomFrictionArchivePrefix verifies LoomFrictionArchivePrefix equals LoomFrictionDir's own
// return value with a trailing hyphen appended, asserted against LoomFrictionDir's own output
// rather than a second hand-built literal, so the two cannot drift.
func TestLoomFrictionArchivePrefix(t *testing.T) {
	l := subpathLocation()

	want := LoomFrictionDir(l) + "-"
	if got := LoomFrictionArchivePrefix(l); got != want {
		t.Errorf("LoomFrictionArchivePrefix() = %q; want %q", got, want)
	}
}

// TestLoomFrictionLock verifies the reflection lock stays ephemeral: under .lyx/loom, with its parent equal to LoomScratchDir.
func TestLoomFrictionLock(t *testing.T) {
	l := subpathLocation()

	want := filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, "loom", "friction.lock")
	if got := LoomFrictionLock(l); got != want {
		t.Errorf("LoomFrictionLock() = %q; want %q", got, want)
	}
	if got := LoomFrictionLock(l); filepath.Dir(got) != LoomScratchDir(l) {
		t.Errorf("LoomFrictionLock() = %q; want its parent to equal LoomScratchDir() = %q", got, LoomScratchDir(l))
	}
}
