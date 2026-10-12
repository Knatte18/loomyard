// prompt_test.go — untagged Tier-1 tests for the agent's prompt and launch spec, over a stencils directory seeded from the shipped registry.

package alignment

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/editdirective"
	"github.com/Knatte18/loomyard/internal/parentdirective"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
)

// TestPrompt verifies both points render every marker with their own paths, fill the absent inputs as none, leave no unfilled marker and carry both directives.
func TestPrompt(t *testing.T) {
	t.Parallel()
	stencilsDir := t.TempDir()
	stencilkit.SeedInto(t, stencilsDir)

	parentDirective, err := parentdirective.Directive(stencilsDir, "", false)
	if err != nil {
		t.Fatalf("parentdirective.Directive() error = %v; want nil", err)
	}
	editDirective, err := editdirective.Directive(stencilsDir)
	if err != nil {
		t.Fatalf("editdirective.Directive() error = %v; want nil", err)
	}

	base := PromptInput{
		Slug:               "task-a",
		Worktree:           "/hub/task-a",
		DecisionRecordPath: "/hub/task-a/_lyx/discussion/decision-record.md",
		SupportLogPath:     "/hub/task-a/_lyx/discussion/support-log.md",
		PlanDir:            "/hub/task-a/_lyx/plan",
		ParentBranch:       "main",
		VerdictPath:        "/attempt/verdict-tok.yaml",
		ReportPath:         "/attempt/report-tok.md",
	}
	parentReview := base
	parentReview.Point = PointParentReview
	parentReview.BriefPath = "/attempt/brief.md"
	prGate := base
	prGate.Point = PointPRGate
	prGate.PRNumber = 42
	prGate.HeadSHA = "abc1234"

	tests := []struct {
		name     string
		in       PromptInput
		contains []string
	}{
		{"parent-review names its brief and fills the pull request as none", parentReview, []string{"# Alignment for `task-a` at `parent-review`", "/attempt/brief.md", "pull request none at `none`"}},
		{"pr-gate names its pull request and fills the brief as none", prGate, []string{"# Alignment for `task-a` at `pr-gate`", "pull request 42 at `abc1234`", "The brief `none`"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := Prompt(stencilsDir, tt.in)
			if err != nil {
				t.Fatalf("Prompt() error = %v; want nil", err)
			}
			if strings.Contains(got, "{{") {
				t.Errorf("Prompt() leaves an unfilled marker:\n%s", got)
			}
			want := append([]string{
				tt.in.Worktree, tt.in.DecisionRecordPath, tt.in.SupportLogPath, tt.in.PlanDir, tt.in.ParentBranch, tt.in.VerdictPath, tt.in.ReportPath,
				stencilstore.Path(stencilsDir, "burler-review-format"), parentDirective, editDirective,
			}, tt.contains...)
			for _, phrase := range want {
				if !strings.Contains(got, phrase) {
					t.Errorf("Prompt() lacks %q", phrase)
				}
			}
		})
	}

	t.Run("an absent stencil is an error naming it", func(t *testing.T) {
		t.Parallel()
		empty := t.TempDir()
		stencilkit.SeedInto(t, empty)
		stencilkit.Remove(t, empty, stencilName)
		if _, err := Prompt(empty, parentReview); err == nil || !strings.Contains(err.Error(), stencilName) {
			t.Errorf("Prompt() error = %v; want it to name %q", err, stencilName)
		}
	})
}

// TestSpec verifies each field of the launch spec: the prompt verbatim, no skills, both output files in order, the resolved model triple and timeout, the role and segment, and the autonomous no-parent posture.
func TestSpec(t *testing.T) {
	t.Parallel()
	a := Attempt{Dir: filepath.Join("hub", "attempt-1"), Token: "tok"}
	s := Settings{Model: "claude-opus", Effort: "high", Version: "5", Timeout: 20 * time.Minute}

	got := Spec("the prompt", a, s)

	if got.Prompt != "the prompt" || got.Model != "claude-opus" || got.Effort != "high" || got.Version != "5" || got.Timeout != 20*time.Minute {
		t.Errorf("Spec() = %+v; want the prompt verbatim and the settings' model, effort, version and timeout", got)
	}
	if len(got.Skills) != 0 {
		t.Errorf("Spec().Skills = %v; want none", got.Skills)
	}
	if want := []string{a.VerdictPath(), a.ReportPath()}; !reflect.DeepEqual(got.OutputFiles, want) {
		t.Errorf("Spec().OutputFiles = %v; want %v", got.OutputFiles, want)
	}
	if got.Role != Role || got.Segment != segmentcolor.Review || got.NameOverride != "" {
		t.Errorf("Spec() role, segment, name = %q, %q, %q; want %q, %q and no override", got.Role, got.Segment, got.NameOverride, Role, segmentcolor.Review)
	}
	if got.Interactive || got.ForkSubagents || got.Parent != "" {
		t.Errorf("Spec() interactive, fork subagents, parent = %v, %v, %q; want false, false and empty", got.Interactive, got.ForkSubagents, got.Parent)
	}
}
