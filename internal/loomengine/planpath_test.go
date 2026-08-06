// planpath_test.go tests the AnchorPath-anchored PlanDir/PlanOverview accessors on a hand-built lyxcwd.Location — pure path arithmetic, no spawning, untagged (Tier 1).
// It mirrors discussionpath_test.go's construction and AnchorPath-vs-WorktreePath assertion shape.

package loomengine

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

func TestLocationPlanDir(t *testing.T) {
	l := &lyxcwd.Location{
		HubPath:      filepath.Join("home", "user", "repo-HUB"),
		WorktreeName: "repo",
		// AnchorRel deliberately differs from "." to prove the accessor
		// follows the anchored subpath, not the bare worktree root.
		AnchorRel: filepath.Join("sub", "dir"),
	}

	want := filepath.Join(l.AnchorPath(), configengine.LyxDirName, "plan")
	if got := PlanDir(l); got != want {
		t.Errorf("PlanDir() = %q; want %q", got, want)
	}
}

func TestLocationPlanOverview(t *testing.T) {
	l := &lyxcwd.Location{
		HubPath:      filepath.Join("home", "user", "repo-HUB"),
		WorktreeName: "repo",
		AnchorRel:    filepath.Join("sub", "dir"),
	}

	want := filepath.Join(l.AnchorPath(), configengine.LyxDirName, "plan", "00-overview.md")
	if got := PlanOverview(l); got != want {
		t.Errorf("PlanOverview() = %q; want %q", got, want)
	}
}

func TestLocationPlanDir_UnanchoredEqualsWorktreePath(t *testing.T) {
	l := &lyxcwd.Location{
		HubPath:      filepath.Join("home", "user", "repo-HUB"),
		WorktreeName: "repo",
		AnchorRel:    ".",
	}

	want := filepath.Join(l.WorktreePath(), configengine.LyxDirName, "plan")
	if got := PlanDir(l); got != want {
		t.Errorf("PlanDir() = %q; want %q", got, want)
	}
}
