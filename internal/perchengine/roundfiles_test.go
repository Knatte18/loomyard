// roundfiles_test.go table-drives roundToken/artifactPaths' naming shape
// and checks buildRoundProfile's field mapping: every burlerengine content
// field carried 1:1, the loop-owned output paths set from paths, and the
// operator-owned prior lists passed through verbatim rather than invented.

package perchengine

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/burlerengine"
)

func TestRoundToken(t *testing.T) {
	tests := []struct {
		name    string
		round   int
		attempt int
		want    string
	}{
		{"first attempt has no suffix", 3, 1, "3"},
		{"second attempt gets b", 3, 2, "3b"},
		{"third attempt gets c", 3, 3, "3c"},
		{"round one first attempt", 1, 1, "1"},
		{"round ten second attempt", 10, 2, "10b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := roundToken(tt.round, tt.attempt)
			if got != tt.want {
				t.Errorf("roundToken(%d, %d) = %q; want %q", tt.round, tt.attempt, got, tt.want)
			}
		})
	}
}

func TestArtifactPaths(t *testing.T) {
	runDir := filepath.Join("run", "dir")

	tests := []struct {
		name    string
		round   int
		attempt int
		want    roundArtifactPaths
	}{
		{
			name:    "first attempt of round 3",
			round:   3,
			attempt: 1,
			want: roundArtifactPaths{
				Review:      filepath.Join(runDir, "round-3-review.md"),
				FixerReport: filepath.Join(runDir, "round-3-fixer-report.md"),
				Judge:       filepath.Join(runDir, "round-3-judge.md"),
				Gate:        filepath.Join(runDir, "round-3-gate.md"),
				Triage:      filepath.Join(runDir, "round-3-triage.md"),
			},
		},
		{
			name:    "second attempt of round 3",
			round:   3,
			attempt: 2,
			want: roundArtifactPaths{
				Review:      filepath.Join(runDir, "round-3b-review.md"),
				FixerReport: filepath.Join(runDir, "round-3b-fixer-report.md"),
				Judge:       filepath.Join(runDir, "round-3b-judge.md"),
				Gate:        filepath.Join(runDir, "round-3b-gate.md"),
				Triage:      filepath.Join(runDir, "round-3b-triage.md"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := artifactPaths(runDir, tt.round, tt.attempt)
			if got != tt.want {
				t.Errorf("artifactPaths(%q, %d, %d) = %+v; want %+v", runDir, tt.round, tt.attempt, got, tt.want)
			}
		})
	}
}

// TestBuildRoundProfile_FieldMapping asserts every burlerengine content
// field is carried 1:1 from Profile, the loop-owned ReviewPath/
// FixerReportPath come from paths, and the operator-owned prior lists are
// passed through unmodified — buildRoundProfile must never append to or
// otherwise invent entries in them.
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
	paths := artifactPaths(filepath.Join("run", "dir"), 3, 1)
	priorReviews := []string{"round-1-review.md", "round-2-review.md"}
	priorFixerReports := []string{"round-1-fixer-report.md", "round-2-fixer-report.md"}

	got := buildRoundProfile(p, paths, priorReviews, priorFixerReports)

	want := burlerengine.Profile{
		Target:            p.Target,
		Fasit:             p.Fasit,
		Rubric:            p.Rubric,
		FixScope:          p.FixScope,
		ToolUse:           p.ToolUse,
		ClusterFan:        p.ClusterFan,
		ReviewPath:        paths.Review,
		FixerReportPath:   paths.FixerReport,
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

// stringSlicesEqual reports whether a and b contain the same strings in
// the same order.
func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
