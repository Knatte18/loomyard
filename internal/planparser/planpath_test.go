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

func TestArchiveDirName(t *testing.T) {
	tests := []struct {
		name   string
		stamp  string
		suffix string
		want   string
	}{
		{"EmptySuffix", "20260824T170000Z", "", "archive-20260824T170000Z"},
		{"CollisionSuffix", "20260824T170000Z", "-1", "archive-20260824T170000Z-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ArchiveDirName(tt.stamp, tt.suffix); got != tt.want {
				t.Errorf("ArchiveDirName(%q, %q) = %q; want %q", tt.stamp, tt.suffix, got, tt.want)
			}
		})
	}
}
