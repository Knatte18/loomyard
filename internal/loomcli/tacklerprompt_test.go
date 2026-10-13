// tacklerprompt_test.go — untagged Tier-1 tests for the tackler's prompt and launch spec composers.

package loomcli

import (
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/parentdirective"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
)

// TestTacklerPrompt asserts the shipped stencil renders every marker value, the no-parent and edit directives, the literal none for an absent report, and no unfilled marker.
func TestTacklerPrompt(t *testing.T) {
	t.Parallel()

	in := tacklerPromptInput{
		RunID:         "operator-surface",
		Slug:          "operator-surface-task",
		Worktree:      "/hub/wt",
		EnvelopePath:  "/hub/wt/.lyx/envelope.json",
		ReportPath:    "/hub/wt/.lyx/report.md",
		TraceCopy:     "/hub/wt/.lyx/steps/t1.trace.log",
		StderrPath:    "/hub/wt/.lyx/steps/t1.stderr.log",
		TraceDir:      "/hub/wt/.lyx/traces",
		DriverLog:     "/hub/wt/.lyx/driver.log",
		VerdictPath:   "/hub/wt/_lyx/shed/operator-surface/tackler/attempt-1/verdict.yaml",
		CaseBriefPath: "/hub/wt/_lyx/shed/operator-surface/tackler/attempt-1/case-brief.md",
	}
	noReport := in
	noReport.ReportPath = ""
	cases := []struct {
		name string
		in   tacklerPromptInput
		want []string
	}{
		{
			name: "every marker value",
			in:   in,
			want: []string{in.RunID, in.Slug, in.Worktree, in.EnvelopePath, in.ReportPath, in.TraceCopy, in.StderrPath, in.TraceDir, in.DriverLog, in.VerdictPath, in.CaseBriefPath},
		},
		{
			name: "absent report reads none",
			in:   noReport,
			want: []string{"stop report so far `none`"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := seededStencils(t)
			got, err := tacklerPrompt(dir, tc.in)
			if err != nil {
				t.Fatalf("tacklerPrompt() error = %v; want nil", err)
			}
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Errorf("tacklerPrompt() does not carry %q", want)
				}
			}
			directive, err := parentdirective.Directive(dir, "", false)
			if err != nil {
				t.Fatalf("parentdirective.Directive: %v", err)
			}
			if !strings.Contains(got, directive) {
				t.Errorf("tacklerPrompt() does not carry the no-parent directive %q", directive)
			}
			if !strings.Contains(got, "Edit or Write") {
				t.Error("tacklerPrompt() does not carry the edit directive")
			}
			if strings.Contains(got, "{{.") {
				t.Error("tacklerPrompt() left a marker unfilled")
			}
		})
	}
}

// TestTacklerSpec pins tacklerSpec's output shape: the resolved model triple and timeout, the verdict as the single output file, the tackler role, the coordinator segment, and no skills, interactivity or parent.
//
//testtiming:keep pins every field of the tackler launch spec: the verdict as its single output file, the resolved model, effort, version and timeout, the role, the empty skill list and parent, and the zero-valued fields
func TestTacklerSpec(t *testing.T) {
	t.Parallel()

	const prompt = "settle the stop"
	const verdictPath = "/hub/wt/_lyx/shed/run/tackler/attempt-1/verdict.yaml"
	const caseBriefPath = "/hub/wt/_lyx/shed/run/tackler/attempt-1/case-brief.md"
	settings := loomengine.TacklerSettings{Model: "claude-resolved-model-id", Effort: "low", Version: "v9", Timeout: 10 * time.Minute, MaxSpawns: 4}

	got := tacklerSpec(prompt, verdictPath, caseBriefPath, settings)

	if got.Prompt != prompt {
		t.Errorf("tacklerSpec().Prompt = %q; want %q", got.Prompt, prompt)
	}
	if len(got.OutputFiles) != 1 || got.OutputFiles[0] != verdictPath {
		t.Errorf("tacklerSpec().OutputFiles = %v; want [%q]", got.OutputFiles, verdictPath)
	}
	if got.Model != settings.Model || got.Effort != settings.Effort || got.Version != settings.Version {
		t.Errorf("tacklerSpec() model, effort, version = %q, %q, %q; want %q, %q, %q", got.Model, got.Effort, got.Version, settings.Model, settings.Effort, settings.Version)
	}
	if got.Timeout != settings.Timeout {
		t.Errorf("tacklerSpec().Timeout = %v; want %v", got.Timeout, settings.Timeout)
	}
	if got.Role != loomengine.TacklerRole {
		t.Errorf("tacklerSpec().Role = %q; want %q", got.Role, loomengine.TacklerRole)
	}
	if got.NameOverride != "" {
		t.Errorf("tacklerSpec().NameOverride = %q; want empty so a second attempt numbers its role", got.NameOverride)
	}
	if len(got.Skills) != 0 {
		t.Errorf("tacklerSpec().Skills = %v; want none", got.Skills)
	}
	if got.Segment != segmentcolor.Coordinator {
		t.Errorf("tacklerSpec().Segment = %q; want %q", got.Segment, segmentcolor.Coordinator)
	}
	if got.Interactive || got.ForkSubagents || got.Parent != "" {
		t.Errorf("tacklerSpec() interactive, fork subagents, parent = %v, %v, %q; want false, false, empty", got.Interactive, got.ForkSubagents, got.Parent)
	}
	if got.Display != (render.Display{}) {
		t.Errorf("tacklerSpec().Display = %+v; want the zero value", got.Display)
	}
}
