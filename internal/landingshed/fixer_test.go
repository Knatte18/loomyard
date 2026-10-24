//go:build tier0

// fixer_test.go — tier0 unit tests for FixerTable.
// The stencils are the shipped defaults seeded into a t.TempDir() stencils dir;
// the lead's prompt is composed through seatengine.Prompts, which starts nothing.

package landingshed

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
)

func newFixerInputs(t *testing.T, mode FixerMode) FixerInputs {
	t.Helper()
	stencilsDir := t.TempDir()
	stencilkit.SeedInto(t, stencilsDir)
	return FixerInputs{
		StencilsDir:        stencilsDir,
		Mode:               mode,
		DecisionRecordPath: "/x/decision-record.md",
		MergeRecordPath:    "/x/merge-record.json",
		PlanDir:            "/x/plan",
		FailureRecordPath:  "/x/landing-failure.yaml",
		AlignedTip:         "aligned123",
		MergedTip:          "merged456",
		ReportPath:         "/x/merge-in-fixer-report.md",
		TaskBranch:         "task-branch",
		ParentBranch:       "parent-branch",
		Slug:               "my-slug",
		LanguageSkills:     []string{"scribe:golang-build"},
	}
}

func TestFixerTable(t *testing.T) {
	t.Parallel()

	cfg := Config{Fixer: "claude:opus[effort=high]", FixerTimeoutMin: 9}

	for _, mode := range []FixerMode{FixerModeMergeIn, FixerModeFinalize} {
		t.Run("Table_"+string(mode), func(t *testing.T) {
			t.Parallel()
			in := newFixerInputs(t, mode)

			table, err := FixerTable(in, cfg, modelspec.Registry{})
			if err != nil {
				t.Fatalf("FixerTable(%s) = _, %v; want nil error", mode, err)
			}
			if err := table.Validate(in.StencilsDir); err != nil {
				t.Fatalf("table.Validate() = %v; want nil", err)
			}

			lead := table.Chair()
			if len(table.Seats) != 1 || lead.Name != "" {
				t.Errorf("table has %d seats, lead name %q; want the lead alone with no name", len(table.Seats), lead.Name)
			}
			if table.RolePrefix != "fixer" || table.Segment != segmentcolor.Landing || table.Interactive {
				t.Errorf("table role prefix/segment/interactive = %q/%q/%v; want fixer/%q/false", table.RolePrefix, table.Segment, table.Interactive, segmentcolor.Landing)
			}
			if lead.Engine != "claude" || lead.Model != "opus" || lead.Effort != "high" || table.Timeout != 9*time.Minute {
				t.Errorf("lead engine/model/effort, table timeout = %q/%q/%q, %v; want claude/opus/high, 9m", lead.Engine, lead.Model, lead.Effort, table.Timeout)
			}
			if want := []string{in.ReportPath}; !slices.Equal(lead.Outputs, want) {
				t.Errorf("lead outputs = %v; want %v", lead.Outputs, want)
			}
			if !slices.Equal(lead.Skills, in.LanguageSkills) {
				t.Errorf("lead skills = %v; want %v", lead.Skills, in.LanguageSkills)
			}
			if table.Values["mode"] != string(mode) || table.Values["aligned_tip"] != "aligned123" || table.Values["merged_tip"] != "merged456" {
				t.Errorf("table values mode/aligned_tip/merged_tip = %q/%q/%q; want %q/aligned123/merged456", table.Values["mode"], table.Values["aligned_tip"], table.Values["merged_tip"], mode)
			}

			prompt := describeLeadPrompt(t, in.StencilsDir, "ab:cd:webster", table)
			for _, v := range []string{string(mode), in.DecisionRecordPath, in.MergeRecordPath, in.PlanDir, in.FailureRecordPath, in.AlignedTip, in.MergedTip, in.ReportPath, in.TaskBranch, in.ParentBranch, in.Slug} {
				if !strings.Contains(prompt, v) {
					t.Errorf("prompt does not contain input value %q", v)
				}
			}
		})
	}

	t.Run("OptionalValuesRenderEmpty", func(t *testing.T) {
		t.Parallel()
		in := newFixerInputs(t, FixerModeFinalize)
		in.PlanDir = ""
		in.FailureRecordPath = ""
		in.AlignedTip = ""

		table, err := FixerTable(in, cfg, modelspec.Registry{})
		if err != nil {
			t.Fatalf("FixerTable(finalize, empty optionals) = _, %v; want nil error", err)
		}
		if err := table.Validate(in.StencilsDir); err != nil {
			t.Fatalf("table.Validate() = %v; want nil", err)
		}
		for _, name := range []string{"plan_dir", "failure_record_path", "aligned_tip"} {
			if !slices.Contains(table.Optional, name) {
				t.Errorf("finalize table Optional = %v; want it to list %q", table.Optional, name)
			}
		}
		prompt := describeLeadPrompt(t, in.StencilsDir, "ab:cd:webster", table)
		if strings.Contains(prompt, "{{") {
			t.Errorf("prompt carries an unrendered marker: %q", prompt)
		}
	})

	t.Run("MergeInOptionals", func(t *testing.T) {
		t.Parallel()
		in := newFixerInputs(t, FixerModeMergeIn)
		in.PlanDir = ""
		in.FailureRecordPath = ""

		table, err := FixerTable(in, cfg, modelspec.Registry{})
		if err != nil {
			t.Fatalf("FixerTable(merge-in, empty optionals) = _, %v; want nil error", err)
		}
		if want := []string{"plan_dir", "failure_record_path"}; !slices.Equal(table.Optional, want) {
			t.Errorf("merge-in table Optional = %v; want %v", table.Optional, want)
		}
	})

	t.Run("MissingStencil", func(t *testing.T) {
		t.Parallel()
		in := newFixerInputs(t, FixerModeMergeIn)
		if err := os.Remove(filepath.Join(in.StencilsDir, "landing", "landing-template-fixer.md")); err != nil {
			t.Fatalf("os.Remove(fixer stencil) = %v; want nil", err)
		}

		if _, err := FixerTable(in, cfg, modelspec.Registry{}); err == nil || !strings.Contains(err.Error(), "landing-template-fixer") {
			t.Errorf("FixerTable(missing stencil) error = %v; want one naming the stencil", err)
		}
	})

	errTests := []struct {
		name    string
		mutate  func(in *FixerInputs)
		mode    FixerMode
		wantErr string
	}{
		{"EmptyStencilsDir", func(in *FixerInputs) { in.StencilsDir = "" }, FixerModeMergeIn, "StencilsDir is empty"},
		{"EmptyMode", func(in *FixerInputs) { in.Mode = "" }, FixerModeMergeIn, "Mode is empty"},
		{"EmptyDecisionRecordPath", func(in *FixerInputs) { in.DecisionRecordPath = "" }, FixerModeMergeIn, "DecisionRecordPath is empty"},
		{"EmptyMergeRecordPath", func(in *FixerInputs) { in.MergeRecordPath = "" }, FixerModeFinalize, "MergeRecordPath is empty"},
		{"EmptyMergedTip", func(in *FixerInputs) { in.MergedTip = "" }, FixerModeFinalize, "MergedTip is empty"},
		{"EmptyReportPath", func(in *FixerInputs) { in.ReportPath = "" }, FixerModeMergeIn, "ReportPath is empty"},
		{"EmptyTaskBranch", func(in *FixerInputs) { in.TaskBranch = "" }, FixerModeMergeIn, "TaskBranch is empty"},
		{"EmptyParentBranch", func(in *FixerInputs) { in.ParentBranch = "" }, FixerModeMergeIn, "ParentBranch is empty"},
		{"EmptySlug", func(in *FixerInputs) { in.Slug = "" }, FixerModeMergeIn, "Slug is empty"},
		{"UnknownMode", func(in *FixerInputs) { in.Mode = "other" }, FixerModeMergeIn, `unknown Mode "other"`},
		{"MergeInEmptyAlignedTip", func(in *FixerInputs) { in.AlignedTip = "" }, FixerModeMergeIn, "aligned_tip"},
	}
	for _, tt := range errTests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			in := newFixerInputs(t, tt.mode)
			tt.mutate(&in)

			table, err := FixerTable(in, cfg, modelspec.Registry{})
			if err == nil && in.StencilsDir != "" {
				// An empty optional the seat engine judges is refused when the table is validated, not when it is built.
				err = table.Validate(in.StencilsDir)
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("FixerTable(%s) error = %v; want one containing %q", tt.name, err, tt.wantErr)
			}
		})
	}
}
