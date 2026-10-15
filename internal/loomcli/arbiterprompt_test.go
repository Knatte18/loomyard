package loomcli

import (
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/parentdirective"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// TestArbiterPrompt asserts the shipped stencil renders every marker value, the no-parent and edit directives, the literal none for an absent report, brief and case brief, and no unfilled marker.
func TestArbiterPrompt(t *testing.T) {
	t.Parallel()

	in := arbiterPromptInput{
		RunID:              "operator-surface",
		Slug:               "operator-surface-task",
		Worktree:           "/hub/wt",
		EnvelopePath:       "/hub/wt/.lyx/envelope.json",
		ReportPath:         "/hub/wt/.lyx/report.md",
		BriefPath:          "/hub/wt/.lyx/brief.md",
		CaseBriefPath:      "/hub/wt/.lyx/tackler/attempt-1/case-brief.md",
		DecisionRecordPath: "/hub/wt/_lyx/discussion/decision-record.md",
		SupportLogPath:     "/hub/wt/_lyx/discussion/support-log.md",
		VerdictPath:        "/hub/wt/.lyx/arbiter/attempt-1/verdict.yaml",
	}
	cases := []struct {
		name string
		in   arbiterPromptInput
		want []string
		none int
	}{
		{
			name: "every marker value",
			in:   in,
			want: []string{in.RunID, in.Slug, in.Worktree, in.EnvelopePath, in.ReportPath, in.BriefPath, in.CaseBriefPath, in.DecisionRecordPath, in.SupportLogPath, in.VerdictPath},
		},
		{
			name: "absent report, brief and case brief read none",
			in:   arbiterPromptInput{RunID: in.RunID, Slug: in.Slug, Worktree: in.Worktree, EnvelopePath: in.EnvelopePath, DecisionRecordPath: in.DecisionRecordPath, SupportLogPath: in.SupportLogPath, VerdictPath: in.VerdictPath},
			want: []string{"`none`"},
			none: 3,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := seededStencils(t)
			got, err := arbiterPrompt(dir, tc.in)
			if err != nil {
				t.Fatalf("arbiterPrompt() error = %v; want nil", err)
			}
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Errorf("arbiterPrompt() does not carry %q", want)
				}
			}
			if tc.none > 0 {
				for _, line := range []string{"driver's stop or fork report `none`", "escalation brief `none`", "case brief `none`"} {
					if !strings.Contains(got, line) {
						t.Errorf("arbiterPrompt() does not carry %q", line)
					}
				}
			}
			directive, err := parentdirective.Directive(dir, "", false)
			if err != nil {
				t.Fatalf("parentdirective.Directive: %v", err)
			}
			if !strings.Contains(got, directive) {
				t.Errorf("arbiterPrompt() does not carry the no-parent directive %q", directive)
			}
			if !strings.Contains(got, "Edit or Write") {
				t.Error("arbiterPrompt() does not carry the edit directive")
			}
			if strings.Contains(got, "{{") {
				t.Error("arbiterPrompt() left a marker unfilled")
			}
		})
	}
}

// TestArbiterSpec pins arbiterSpec's output shape, each field as its own named assertion.
//
//testtiming:keep pins every field of the arbiter launch spec: the verdict as its single output file, the resolved model, effort, version and timeout, the role, the empty name override and skill list and the zero-valued fields
func TestArbiterSpec(t *testing.T) {
	t.Parallel()

	const prompt = "settle the stop"
	const verdictPath = "/hub/wt/.lyx/arbiter/attempt-1/verdict.yaml"
	settings := loomengine.ArbiterSettings{ModelChoice: shuttleengine.ModelChoice{Engine: "b", Model: "claude-resolved-model-id", Effort: "high", Version: "v9"}, Timeout: 20 * time.Minute, MaxSpawns: 3}

	got := arbiterSpec(prompt, verdictPath, settings)

	if got.Prompt != prompt {
		t.Errorf("arbiterSpec().Prompt = %q; want %q", got.Prompt, prompt)
	}
	if len(got.OutputFiles) != 1 || got.OutputFiles[0] != verdictPath {
		t.Errorf("arbiterSpec().OutputFiles = %v; want [%q]", got.OutputFiles, verdictPath)
	}
	if got.ModelChoice != settings.ModelChoice {
		t.Errorf("arbiterSpec().ModelChoice = %+v; want %+v", got.ModelChoice, settings.ModelChoice)
	}
	if got.Timeout != settings.Timeout {
		t.Errorf("arbiterSpec().Timeout = %v; want %v", got.Timeout, settings.Timeout)
	}
	if got.Role != loomengine.ArbiterRole {
		t.Errorf("arbiterSpec().Role = %q; want %q", got.Role, loomengine.ArbiterRole)
	}
	if got.NameOverride != "" {
		t.Errorf("arbiterSpec().NameOverride = %q; want empty so a second attempt numbers its role", got.NameOverride)
	}
	if len(got.Skills) != 0 {
		t.Errorf("arbiterSpec().Skills = %v; want none", got.Skills)
	}
	if got.Segment != segmentcolor.Coordinator {
		t.Errorf("arbiterSpec().Segment = %q; want %q", got.Segment, segmentcolor.Coordinator)
	}
	if len(got.Requires) != 0 {
		t.Errorf("arbiterSpec().Requires = %v; want none", got.Requires)
	}
	if got.Interactive || got.ForkSubagents || got.Parent != "" {
		t.Errorf("arbiterSpec() interactive, fork subagents, parent = %v, %v, %q; want false, false, empty", got.Interactive, got.ForkSubagents, got.Parent)
	}
	if got.Display != (render.Display{}) {
		t.Errorf("arbiterSpec().Display = %+v; want the zero value", got.Display)
	}
}
