// adapter_test.go checks buildRoundProfile's field mapping: every
// burlerengine content field carried 1:1 from Profile, the loop-owned
// ReviewPath/FixerReportPath set from the caller-supplied path strings, and
// the operator-owned prior lists passed through verbatim rather than
// invented. Extracted from roundfiles_test.go (now
// internal/treadleengine/roundfiles_test.go) when buildRoundProfile itself
// moved here as the burler adapter's own field-mapping logic; its subject —
// buildRoundProfile — lives in adapter.go.

package perchengine

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/burlerengine"
)

// TestBuildRoundProfile_FieldMapping asserts every burlerengine content
// field is carried 1:1 from Profile, the loop-owned ReviewPath/
// FixerReportPath come from the caller-supplied path strings, and the
// operator-owned prior lists are passed through unmodified —
// buildRoundProfile must never append to or otherwise invent entries in
// them.
func TestBuildRoundProfile_FieldMapping(t *testing.T) {
	p := Profile{
		Target:     burlerengine.FileSet{Paths: []string{"target.txt"}},
		Fasit:      burlerengine.FileSet{Instructions: "judge against the discussion"},
		Rubric:     "the widget must be blue",
		FixScope:   burlerengine.FixScopeSource,
		ToolUse:    true,
		ClusterFan: "standard",
		// Perch-owned fields must never leak into the burler round profile.
		Gate:        Gate{Mode: GateLLMVerdict},
		RoundCaps:   []int{5, 8, 10},
		JudgeModel:  "haiku",
		JudgeEffort: "low",
		Model:       "opus",
		Effort:      "high",
	}
	// buildRoundProfile's post-extraction signature takes plain path
	// strings (the roundArtifactPaths type stays unexported inside
	// treadleengine) — literal paths stand in for what treadleengine's
	// AttemptInput would supply.
	reviewPath := filepath.Join("run", "dir", "round-3-review.md")
	fixerReportPath := filepath.Join("run", "dir", "round-3-fixer-report.md")
	priorReviews := []string{"round-1-review.md", "round-2-review.md"}
	priorFixerReports := []string{"round-1-fixer-report.md", "round-2-fixer-report.md"}

	got := buildRoundProfile(p, reviewPath, fixerReportPath, priorReviews, priorFixerReports)

	want := burlerengine.Profile{
		Target:            p.Target,
		Fasit:             p.Fasit,
		Rubric:            p.Rubric,
		FixScope:          p.FixScope,
		ToolUse:           p.ToolUse,
		ClusterFan:        p.ClusterFan,
		ReviewPath:        reviewPath,
		FixerReportPath:   fixerReportPath,
		PriorReviews:      priorReviews,
		PriorFixerReports: priorFixerReports,
	}

	if got.Target.Instructions != want.Target.Instructions || len(got.Target.Paths) != len(want.Target.Paths) {
		t.Errorf("Target = %+v; want %+v", got.Target, want.Target)
	}
	if got.Fasit.Instructions != want.Fasit.Instructions {
		t.Errorf("Fasit = %+v; want %+v", got.Fasit, want.Fasit)
	}
	if got.Rubric != want.Rubric {
		t.Errorf("Rubric = %q; want %q", got.Rubric, want.Rubric)
	}
	if got.FixScope != want.FixScope {
		t.Errorf("FixScope = %q; want %q", got.FixScope, want.FixScope)
	}
	if got.ToolUse != want.ToolUse {
		t.Errorf("ToolUse = %v; want %v", got.ToolUse, want.ToolUse)
	}
	if got.ClusterFan != want.ClusterFan {
		t.Errorf("ClusterFan = %q; want %q", got.ClusterFan, want.ClusterFan)
	}
	if got.ReviewPath != want.ReviewPath {
		t.Errorf("ReviewPath = %q; want %q", got.ReviewPath, want.ReviewPath)
	}
	if got.FixerReportPath != want.FixerReportPath {
		t.Errorf("FixerReportPath = %q; want %q", got.FixerReportPath, want.FixerReportPath)
	}
	if !stringSlicesEqual(got.PriorReviews, priorReviews) {
		t.Errorf("PriorReviews = %v; want %v", got.PriorReviews, priorReviews)
	}
	if !stringSlicesEqual(got.PriorFixerReports, priorFixerReports) {
		t.Errorf("PriorFixerReports = %v; want %v", got.PriorFixerReports, priorFixerReports)
	}
}
