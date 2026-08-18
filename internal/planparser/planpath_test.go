// planpath_test.go tests the told-anchor PlanDir/PlanOverview path constructors — pure filepath.Join
// arithmetic, no spawning, no fixture tree, untagged (Tier 1). It is the ported successor of
// internal/loomengine/planpath_test.go (deleted in batch 2).

package planparser

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

func TestPlanDir(t *testing.T) {
	anchor := filepath.Join("home", "user", "repo", "sub", "dir")

	want := filepath.Join(anchor, lyxdirs.LyxDirName, "plan")
	if got := PlanDir(anchor); got != want {
		t.Errorf("PlanDir(%q) = %q; want %q", anchor, got, want)
	}
}

func TestPlanOverview(t *testing.T) {
	anchor := filepath.Join("home", "user", "repo", "sub", "dir")

	want := filepath.Join(PlanDir(anchor), "00-overview.md")
	if got := PlanOverview(anchor); got != want {
		t.Errorf("PlanOverview(%q) = %q; want %q", anchor, got, want)
	}

	wantFromConstant := filepath.Join(PlanDir(anchor), overviewFileName)
	if got := PlanOverview(anchor); got != wantFromConstant {
		t.Errorf("PlanOverview(%q) = %q; want %q (via overviewFileName)", anchor, got, wantFromConstant)
	}
}
