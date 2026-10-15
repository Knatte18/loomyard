// tacklerprompt_test.go — untagged Tier-1 tests for the tackler's prompt and launch spec composers.

package sheddriver

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/parentdirective"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
)

// TestTacklerPrompt asserts the shipped stencil renders every marker value, the no-parent and edit directives, the literal none for an absent report, the recipe line under its heading, the shed-layer wording, and no unfilled marker; it refuses an empty recipe or goto rows naming the marker.
func TestTacklerPrompt(t *testing.T) {
	t.Parallel()

	in := TacklerPromptInput{
		RunID:         "operator-surface",
		Slug:          "operator-surface-task",
		Recipe:        "loom: a phased task run (discussion, plan, webster's coded batches, reviews, landing)",
		GotoRows:      "`Plan-Bouncer`",
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
	routedFrom := in
	routedFrom.GotoRows = "`Plan-Bouncer` or `Plan-Write`"
	noRecipe := in
	noRecipe.Recipe = ""
	noGotoRows := in
	noGotoRows.GotoRows = ""
	cases := []struct {
		name string
		in   TacklerPromptInput
		want []string
		// wantAbsent are phrases the rendered prompt must not carry.
		wantAbsent []string
		// wantErr is the text the refusal names; empty rows expect a prompt.
		wantErr string
	}{
		{
			name: "every marker value",
			in:   in,
			want: []string{in.RunID, in.Slug, in.Recipe, in.GotoRows, in.Worktree, in.EnvelopePath, in.ReportPath, in.TraceCopy, in.StderrPath, in.TraceDir, in.DriverLog, in.VerdictPath, in.CaseBriefPath},
		},
		{
			name: "absent report reads none",
			in:   noReport,
			want: []string{"stop report so far `none`"},
		},
		{
			name: "the recipe line is filled under the run heading",
			in:   in,
			want: []string{"## The run\n\n" + in.Recipe},
		},
		{
			name:       "the prompt is shed-layer wording with the stop kinds, the reading order and a two-path write rule",
			in:         in,
			want:       []string{"transient", "`producer`", "`bootstrap`", "`unseeded`", "`interrupted`", "`blocked`", "only when those leave the cause open", "Write `" + in.VerdictPath + "` and, on an `escalate`, `" + in.CaseBriefPath + "`, and nothing else."},
			wantAbsent: []string{"arbiter", "Arbiter"},
		},
		{
			name:       "the goto rows are the halted row alone unless told more",
			in:         in,
			want:       []string{"the only rows it may name are `Plan-Bouncer`."},
			wantAbsent: []string{"Plan-Write"},
		},
		{
			name: "a told routed-from row is named beside the current row",
			in:   routedFrom,
			want: []string{"the only rows it may name are `Plan-Bouncer` or `Plan-Write`."},
		},
		{name: "an empty recipe is refused naming its marker", in: noRecipe, wantErr: "recipe"},
		{name: "empty goto rows are refused naming their marker", in: noGotoRows, wantErr: "goto_rows"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := filepath.Join(t.TempDir(), "stencils")
			stencilkit.SeedInto(t, dir)
			got, err := tacklerPrompt(dir, tc.in)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("tacklerPrompt() error = %v; want it to name %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("tacklerPrompt() error = %v; want nil", err)
			}
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Errorf("tacklerPrompt() does not carry %q", want)
				}
			}
			for _, absent := range tc.wantAbsent {
				if strings.Contains(got, absent) {
					t.Errorf("tacklerPrompt() carries %q; want it absent", absent)
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

// TestTacklerSpec pins tacklerSpec's output shape: the resolved model choice, engine included, and timeout, the verdict as the single output file, the tackler role, the coordinator segment, and no skills, interactivity or parent.
//
//testtiming:keep pins every field of the tackler launch spec: the verdict as its single output file, the resolved model, effort, version and timeout, the role, the empty skill list and parent, and the zero-valued fields
func TestTacklerSpec(t *testing.T) {
	t.Parallel()

	const prompt = "settle the stop"
	const verdictPath = "/hub/wt/_lyx/shed/run/tackler/attempt-1/verdict.yaml"
	const caseBriefPath = "/hub/wt/_lyx/shed/run/tackler/attempt-1/case-brief.md"
	settings := TacklerSettings{ModelChoice: shuttleengine.ModelChoice{Engine: "b", Model: "claude-resolved-model-id", Effort: "low", Version: "v9"}, Timeout: 10 * time.Minute, MaxSpawns: 4}

	got := tacklerSpec(prompt, verdictPath, caseBriefPath, settings)

	if got.Prompt != prompt {
		t.Errorf("tacklerSpec().Prompt = %q; want %q", got.Prompt, prompt)
	}
	if len(got.OutputFiles) != 1 || got.OutputFiles[0] != verdictPath {
		t.Errorf("tacklerSpec().OutputFiles = %v; want [%q]", got.OutputFiles, verdictPath)
	}
	if got.ModelChoice != settings.ModelChoice {
		t.Errorf("tacklerSpec().ModelChoice = %+v; want %+v", got.ModelChoice, settings.ModelChoice)
	}
	if got.Timeout != settings.Timeout {
		t.Errorf("tacklerSpec().Timeout = %v; want %v", got.Timeout, settings.Timeout)
	}
	if got.Role != TacklerRole {
		t.Errorf("tacklerSpec().Role = %q; want %q", got.Role, TacklerRole)
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
	if len(got.Requires) != 0 {
		t.Errorf("tacklerSpec().Requires = %v; want none", got.Requires)
	}
	if got.Interactive || got.ForkSubagents || got.Parent != "" {
		t.Errorf("tacklerSpec() interactive, fork subagents, parent = %v, %v, %q; want false, false, empty", got.Interactive, got.ForkSubagents, got.Parent)
	}
	if got.Display != (render.Display{}) {
		t.Errorf("tacklerSpec().Display = %+v; want the zero value", got.Display)
	}
}
