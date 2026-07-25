// planpath_test.go tests the WorktreeRoot-anchored PlanDir/PlanOverview
// accessors on a hand-built Layout — pure path arithmetic, no spawning,
// untagged (Tier 1). It mirrors discussionpath_test.go's construction and
// WorktreeRoot-vs-Cwd assertion shape.

package hubgeometry

import (
	"path/filepath"
	"testing"
)

func TestLayoutPlanDir(t *testing.T) {
	l := &Layout{
		WorktreeRoot: filepath.Join("home", "user", "repo"),
		// Cwd deliberately differs from WorktreeRoot to prove the accessor
		// ignores Cwd and stays anchored to WorktreeRoot.
		Cwd: filepath.Join("home", "user", "repo", "sub", "dir"),
	}

	want := filepath.Join(l.WorktreeRoot, LyxDirName, "plan")
	if got := l.PlanDir(); got != want {
		t.Errorf("PlanDir() = %q; want %q", got, want)
	}
}

func TestLayoutPlanOverview(t *testing.T) {
	l := &Layout{
		WorktreeRoot: filepath.Join("home", "user", "repo"),
		// Cwd deliberately differs from WorktreeRoot to prove the accessor
		// ignores Cwd and stays anchored to WorktreeRoot.
		Cwd: filepath.Join("home", "user", "repo", "sub", "dir"),
	}

	want := filepath.Join(l.WorktreeRoot, LyxDirName, "plan", "00-overview.md")
	if got := l.PlanOverview(); got != want {
		t.Errorf("PlanOverview() = %q; want %q", got, want)
	}
}

func TestLayoutPlanDir_CwdEqualsWorktreeRoot(t *testing.T) {
	l := &Layout{
		WorktreeRoot: filepath.Join("home", "user", "repo"),
		Cwd:          filepath.Join("home", "user", "repo"),
	}

	want := filepath.Join(l.WorktreeRoot, LyxDirName, "plan")
	if got := l.PlanDir(); got != want {
		t.Errorf("PlanDir() = %q; want %q", got, want)
	}
}
