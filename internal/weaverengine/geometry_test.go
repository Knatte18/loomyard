// geometry_test.go pins Weaver's directory accessors and the geometry's mapping onto the shared machinery.

package weaverengine_test

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/weaverengine"
)

func TestPaths(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	anchor := t.TempDir()
	runDir := weaverengine.Dir(anchor)
	scratch := weaverengine.ScratchDir(anchor)

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"Dir", runDir, filepath.Join(anchor, "_lym", "weaver")},
		{"DirRel", weaverengine.DirRel(), "_lym/weaver"},
		{"ReportsDir", weaverengine.ReportsDir(anchor), filepath.Join(runDir, "reports")},
		{"ScratchDir", scratch, filepath.Join(anchor, ".lym", "weaver")},
		{"PromptsDir", weaverengine.PromptsDir(anchor), filepath.Join(scratch, "prompts")},
		{"JournalPath", weaverengine.JournalPath(runDir), filepath.Join(runDir, "journal.md")},
		{"OutcomePath", weaverengine.OutcomePath(runDir), filepath.Join(runDir, "outcome.yaml")},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %q; want %q", tt.name, tt.got, tt.want)
		}
	}

	g := weaverengine.Geometry{
		WorktreeRoot: "worktree",
		PaneCwd:      "pane",
		PlanDir:      "plan",
		RunDir:       runDir,
		ReportsDir:   weaverengine.ReportsDir(anchor),
		PromptsDir:   weaverengine.PromptsDir(anchor),
		ScratchDir:   scratch,
	}
	rp := weaverengine.RunPlanOf(g)
	if rp.WorktreeRoot != g.WorktreeRoot || rp.PaneCwd != g.PaneCwd || rp.PlanDir != g.PlanDir ||
		rp.RunDir != g.RunDir || rp.ReportsDir != g.ReportsDir || rp.PromptsDir != g.PromptsDir || rp.ScratchDir != g.ScratchDir {
		t.Errorf("runPlan() = %+v; want every path of %+v", rp, g)
	}
	if rp.Git == nil {
		t.Error("runPlan().Git = nil; want the real Git when the geometry carries none")
	}
}
