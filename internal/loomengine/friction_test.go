// friction_test.go — untagged Tier-1 unit tests for LoomFrictionDir and
// LoomFrictionArchivePrefix.
// Mirrors review_test.go's TestLoomReviewsDir shape: pure path arithmetic over a hand-built
// lyxcwd.Location, no live hub, reed, or network involved.

package loomengine

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

// TestLoomFrictionDir verifies LoomFrictionDir's returned path is AnchorPath-anchored, sits under
// the ephemeral .lyx tree rather than the durable one, and mirrors the loom subdirectory
// LoomScratchDir already names.
func TestLoomFrictionDir(t *testing.T) {
	l := &lyxcwd.Location{
		HubPath:      filepath.Join("home", "user", "repo-HUB"),
		WorktreeName: "repo",
		// AnchorRel deliberately differs from "." to prove the accessor follows the
		// anchored subpath, not the bare worktree root.
		AnchorRel: filepath.Join("sub", "dir"),
	}

	want := filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, "loom", "friction")
	if got := LoomFrictionDir(l); got != want {
		t.Errorf("LoomFrictionDir() = %q; want %q", got, want)
	}

	if got := LoomFrictionDir(l); filepath.Dir(got) != LoomScratchDir(l) {
		t.Errorf("LoomFrictionDir() = %q; want its parent to equal LoomScratchDir() = %q", got, LoomScratchDir(l))
	}
}

// TestLoomFrictionArchivePrefix verifies LoomFrictionArchivePrefix equals LoomFrictionDir's own
// return value with a trailing hyphen appended, asserted against LoomFrictionDir's own output
// rather than a second hand-built literal, so the two cannot drift.
func TestLoomFrictionArchivePrefix(t *testing.T) {
	l := &lyxcwd.Location{
		HubPath:      filepath.Join("home", "user", "repo-HUB"),
		WorktreeName: "repo",
		AnchorRel:    filepath.Join("sub", "dir"),
	}

	want := LoomFrictionDir(l) + "-"
	if got := LoomFrictionArchivePrefix(l); got != want {
		t.Errorf("LoomFrictionArchivePrefix() = %q; want %q", got, want)
	}
}
