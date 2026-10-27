//go:build tier0

package shedarbiter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/parentdirective"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/sheddriver"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
)

// seededStencils returns a stencils directory seeded with the shipped stencils.
func seededStencils(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "stencils")
	stencilkit.SeedInto(t, dir)
	return dir
}

// TestPrompt asserts the shipped stencil renders every marker value, the no-parent and edit directives, the recipe description, the literal none for an absent report, brief, case brief, amendment target, decision record, support log and recipe, no unfilled marker and a verdict example the verdict reader accepts.
func TestPrompt(t *testing.T) {
	t.Parallel()

	const battenRecipe = "This is a batten run; it has no repair verb."
	in := PromptInput{
		Recipe:             "This is a loom run. Its repair verbs: `lym loom decision add <slug>`.",
		RunID:              "operator-surface",
		Slug:               "operator-surface-task",
		Worktree:           "/hub/wt",
		EnvelopePath:       "/hub/wt/.lym/envelope.json",
		ReportPath:         "/hub/wt/.lym/report.md",
		BriefPath:          "/hub/wt/.lym/brief.md",
		CaseBriefPath:      "/hub/wt/.lym/tackler/attempt-1/case-brief.md",
		DecisionRecordPath: "/hub/wt/_lym/discussion/decision-record.md",
		SupportLogPath:     "/hub/wt/_lym/discussion/support-log.md",
		VerdictPath:        "/hub/wt/.lym/arbiter/attempt-1/verdict.yaml",
		AmendmentTarget:    "Plan-Burler-Review",
	}
	batten := in
	batten.Recipe = battenRecipe
	absent := PromptInput{RunID: in.RunID, Slug: in.Slug, Worktree: in.Worktree, EnvelopePath: in.EnvelopePath, VerdictPath: in.VerdictPath}
	cases := []struct {
		name string
		in   PromptInput
		want []string
		// absent lists the lines that must read none.
		absent []string
	}{
		{
			name: "every marker value",
			in:   in,
			want: []string{in.Recipe, in.RunID, in.Slug, in.Worktree, in.EnvelopePath, in.ReportPath, in.BriefPath, in.CaseBriefPath, in.DecisionRecordPath, in.SupportLogPath, in.VerdictPath, "amendment target `" + in.AmendmentTarget + "`"},
		},
		{
			name: "a batten description fills the recipe marker",
			in:   batten,
			want: []string{battenRecipe},
		},
		{
			name: "absent recipe, report, brief, case brief, amendment target, decision record and support log read none",
			in:   absent,
			want: []string{"## The run\n\nnone\n"},
			absent: []string{
				"driver's stop or fork report `none`", "escalation brief `none`", "case brief `none`", "amendment target `none`",
				"decision record `none`", "support log `none`",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := seededStencils(t)
			got, err := Prompt(dir, tc.in)
			if err != nil {
				t.Fatalf("Prompt() error = %v; want nil", err)
			}
			for _, want := range append(tc.want, tc.absent...) {
				if !strings.Contains(got, want) {
					t.Errorf("Prompt() does not carry %q", want)
				}
			}
			directive, err := parentdirective.Directive(dir, "", false)
			if err != nil {
				t.Fatalf("parentdirective.Directive: %v", err)
			}
			if !strings.Contains(got, directive) {
				t.Errorf("Prompt() does not carry the no-parent directive %q", directive)
			}
			if !strings.Contains(got, "Edit or Write") {
				t.Error("Prompt() does not carry the edit directive")
			}
			if strings.Contains(got, "{{") {
				t.Error("Prompt() left a marker unfilled")
			}
			_, example, found := strings.Cut(got, "```yaml\n")
			example, _, closed := strings.Cut(example, "```")
			if !found || !closed {
				t.Fatal("Prompt() carries no fenced yaml verdict example")
			}
			verdictPath := filepath.Join(t.TempDir(), "verdict.yaml")
			if err := os.WriteFile(verdictPath, []byte(example), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, _, err := loomengine.ReadArbiterVerdict(verdictPath); err != nil {
				t.Errorf("ReadArbiterVerdict(the prompt's verdict example) error = %v; want nil", err)
			}
		})
	}
}

// TestSpec pins Spec's output shape, each field as its own named assertion.
//
//testtiming:keep pins every field of the arbiter launch spec: the verdict as its single output file, the resolved model, effort, version and timeout, the role, the empty name override and skill list and the zero-valued fields
func TestSpec(t *testing.T) {
	t.Parallel()

	const prompt = "settle the stop"
	const verdictPath = "/hub/wt/.lym/arbiter/attempt-1/verdict.yaml"
	settings := sheddriver.ArbiterSettings{ModelChoice: shuttleengine.ModelChoice{Engine: "b", Model: "claude-resolved-model-id", Effort: "high", Version: "v9"}, Timeout: 20 * time.Minute, MaxSpawns: 3}

	got := Spec(prompt, verdictPath, settings)

	if got.Prompt != prompt {
		t.Errorf("Spec().Prompt = %q; want %q", got.Prompt, prompt)
	}
	if len(got.OutputFiles) != 1 || got.OutputFiles[0] != verdictPath {
		t.Errorf("Spec().OutputFiles = %v; want [%q]", got.OutputFiles, verdictPath)
	}
	if got.ModelChoice != settings.ModelChoice {
		t.Errorf("Spec().ModelChoice = %+v; want %+v", got.ModelChoice, settings.ModelChoice)
	}
	if got.Timeout != settings.Timeout {
		t.Errorf("Spec().Timeout = %v; want %v", got.Timeout, settings.Timeout)
	}
	if got.Role != loomengine.ArbiterRole {
		t.Errorf("Spec().Role = %q; want %q", got.Role, loomengine.ArbiterRole)
	}
	if got.NameOverride != "" {
		t.Errorf("Spec().NameOverride = %q; want empty so a second attempt numbers its role", got.NameOverride)
	}
	if len(got.Skills) != 0 {
		t.Errorf("Spec().Skills = %v; want none", got.Skills)
	}
	if got.Segment != segmentcolor.Coordinator {
		t.Errorf("Spec().Segment = %q; want %q", got.Segment, segmentcolor.Coordinator)
	}
	if len(got.Requires) != 0 {
		t.Errorf("Spec().Requires = %v; want none", got.Requires)
	}
	if got.Interactive || got.ForkSubagents || got.Parent != "" {
		t.Errorf("Spec() interactive, fork subagents, parent = %v, %v, %q; want false, false, empty", got.Interactive, got.ForkSubagents, got.Parent)
	}
	if got.Display != (render.Display{}) {
		t.Errorf("Spec().Display = %+v; want the zero value", got.Display)
	}
}
