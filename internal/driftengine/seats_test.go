// seats_test.go — tier 0 tests for the expect and judge seat tables.
// They run over a seeded stencils directory and a model registry of the built-in aliases,
// and they check the prompts the seat engine composes without starting anything.

package driftengine

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// seatRegistry loads an empty-directory model registry, which still resolves the built-in aliases.
func seatRegistry(t *testing.T) modelspec.Registry {
	t.Helper()
	reg, err := modelspec.LoadRegistry(configengine.Dirs{Anchor: t.TempDir()})
	if err != nil {
		t.Fatalf("modelspec.LoadRegistry() = _, %v; want nil error", err)
	}
	return reg
}

// seatPrompt returns the prompt of table's lead composed over stencilsDir with a parent named.
func seatPrompt(t *testing.T, stencilsDir, worktreeRoot, slug string, table seatengine.Table) string {
	t.Helper()
	prompts, err := seatengine.Prompts(seatengine.Geometry{
		WorktreeRoot: worktreeRoot,
		AnchorPath:   worktreeRoot,
		StencilsDir:  stencilsDir,
		ParentName:   "the-parent",
		Shortname:    "ly",
		Slug:         slug,
	}, table)
	if err != nil {
		t.Fatalf("seatengine.Prompts() = _, %v; want nil error", err)
	}
	return prompts[""]
}

// TestSeatTables covers the two builders.
// It checks each missing required input is refused by name, and the lead's model, role, output, skills and timeout.
// It also checks the markers each prompt renders and the handling of an unparsable model.
func TestSeatTables(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	cfg := Config{ExpectModel: "sonnet[effort=medium]", JudgeModel: "opus[effort=high]"}

	expect := func(t *testing.T) ExpectInputs {
		t.Helper()
		dir := t.TempDir()
		return ExpectInputs{
			Subject:            SubjectDecisionRecord,
			Slug:               "fix-the-thing",
			DecisionRecordPath: filepath.Join(dir, "decision-record.md"),
			PlanDir:            filepath.Join(dir, "plan"),
			BaselineDir:        filepath.Join(dir, "baseline"),
			ListingPath:        filepath.Join(dir, "listing.yaml"),
			ExpectedPath:       filepath.Join(dir, "expected.yaml"),
			DiffMode:           DiffLines,
		}
	}

	t.Run("the expect table refuses a missing input, an unknown mode and an unparsable model", func(t *testing.T) {
		t.Parallel()
		rows := []struct {
			want  string
			clear func(*ExpectInputs)
		}{
			{"DecisionRecordPath", func(in *ExpectInputs) { in.DecisionRecordPath = "" }},
			{"BaselineDir", func(in *ExpectInputs) { in.BaselineDir = "" }},
			{"ListingPath", func(in *ExpectInputs) { in.ListingPath = "" }},
			{"ExpectedPath", func(in *ExpectInputs) { in.ExpectedPath = "" }},
			{"DiffMode", func(in *ExpectInputs) { in.DiffMode = "other" }},
			{"Slug", func(in *ExpectInputs) { in.Subject, in.Slug = SubjectBoardEntry, "" }},
		}
		for _, row := range rows {
			in := expect(t)
			row.clear(&in)
			if _, err := ExpectTable(in, cfg, seatRegistry(t)); err == nil || !strings.Contains(err.Error(), row.want) {
				t.Errorf("ExpectTable() with %s unusable = %v; want a refusal naming it", row.want, err)
			}
		}
		bad := Config{ExpectModel: "not a spec["}
		if _, err := ExpectTable(expect(t), bad, seatRegistry(t)); err == nil || !strings.Contains(err.Error(), "model-spec") {
			t.Errorf("ExpectTable() with an unparsable model = %v; want a model-spec refusal", err)
		}
	})

	t.Run("the expect lead and its prompt", func(t *testing.T) {
		t.Parallel()
		stencilsDir, worktreeRoot := stencilkit.Seed(t), t.TempDir()
		rows := []struct {
			name   string
			edit   func(*ExpectInputs)
			want   []string
			absent []string
		}{
			{"lines with a plan", func(*ExpectInputs) {}, []string{"mode: lines", "range: ", "the plan in `"}, []string{"mode: command", "has no plan"}},
			{"command with no plan", func(in *ExpectInputs) { in.DiffMode, in.PlanDir = DiffCommand, "" }, []string{"mode: command", "has no plan"}, []string{"mode: lines", "the plan in `"}},
		}
		for _, row := range rows {
			t.Run(row.name, func(t *testing.T) {
				t.Parallel()
				in := expect(t)
				row.edit(&in)
				table, err := ExpectTable(in, cfg, seatRegistry(t))
				if err != nil {
					t.Fatalf("ExpectTable() = _, %v; want nil error", err)
				}
				if err := table.Validate(stencilsDir, ""); err != nil {
					t.Fatalf("table.Validate() = %v; want nil", err)
				}
				lead := table.Chair()
				if len(table.Seats) != 1 || lead.Name != "" || table.RolePrefix != "drift-expect" || table.Segment != segmentcolor.Plan || table.Timeout != expectTimeout || table.Interactive {
					t.Errorf("table = %d seats, lead %q, role %q, segment %q, timeout %v, interactive %v; want the lone non-interactive lead of role drift-expect on the plan segment", len(table.Seats), lead.Name, table.RolePrefix, table.Segment, table.Timeout, table.Interactive)
				}
				if lead.Engine != "claude" || lead.Model != "sonnet" || lead.Effort != "medium" {
					t.Errorf("lead engine/model/effort = %q/%q/%q; want claude/sonnet/medium", lead.Engine, lead.Model, lead.Effort)
				}
				if !slices.Equal(lead.Outputs, []string{in.ExpectedPath}) || !slices.Equal(lead.Skills, []string{"scribe:prose"}) {
					t.Errorf("lead outputs %v and skills %v; want expected.yaml alone and scribe:prose", lead.Outputs, lead.Skills)
				}

				prompt := seatPrompt(t, stencilsDir, worktreeRoot, in.Slug, table)
				for _, want := range append([]string{in.ExpectedPath, in.ListingPath, in.BaselineDir, in.DecisionRecordPath, "the-parent"}, row.want...) {
					if !strings.Contains(prompt, want) {
						t.Errorf("expect prompt lacks %q", want)
					}
				}
				for _, absent := range row.absent {
					if strings.Contains(prompt, absent) {
						t.Errorf("expect prompt holds %q", absent)
					}
				}
			})
		}
	})

	judge := func(t *testing.T) JudgeInputs {
		t.Helper()
		dir := t.TempDir()
		return JudgeInputs{
			Subject:            SubjectDecisionRecord,
			Slug:               "fix-the-thing",
			DecisionRecordPath: filepath.Join(dir, "decision-record.md"),
			RequestPath:        filepath.Join(dir, "judge-request.json"),
			JudgementPath:      filepath.Join(dir, "judgement.yaml"),
		}
	}

	t.Run("the judge table refuses a missing input and an unparsable model", func(t *testing.T) {
		t.Parallel()
		rows := []struct {
			want  string
			clear func(*JudgeInputs)
		}{
			{"DecisionRecordPath", func(in *JudgeInputs) { in.DecisionRecordPath = "" }},
			{"RequestPath", func(in *JudgeInputs) { in.RequestPath = "" }},
			{"JudgementPath", func(in *JudgeInputs) { in.JudgementPath = "" }},
			{"Slug", func(in *JudgeInputs) { in.Subject, in.Slug = SubjectBoardEntry, "" }},
		}
		for _, row := range rows {
			in := judge(t)
			row.clear(&in)
			if _, err := JudgeTable(in, cfg, seatRegistry(t)); err == nil || !strings.Contains(err.Error(), row.want) {
				t.Errorf("JudgeTable() with %s unusable = %v; want a refusal naming it", row.want, err)
			}
		}
		bad := Config{JudgeModel: "not a spec["}
		if _, err := JudgeTable(judge(t), bad, seatRegistry(t)); err == nil || !strings.Contains(err.Error(), "model-spec") {
			t.Errorf("JudgeTable() with an unparsable model = %v; want a model-spec refusal", err)
		}
	})

	t.Run("the judge lead and its prompt, autonomous and with the operator", func(t *testing.T) {
		t.Parallel()
		stencilsDir, worktreeRoot := stencilkit.Seed(t), t.TempDir()
		operatorCfg := cfg
		operatorCfg.JudgeInteractive = true
		rows := []struct {
			name        string
			cfg         Config
			interactive bool
			want        string
			absent      string
		}{
			{"autonomous", cfg, false, autonomousJudgeMode, "present each cell or case"},
			{"with the operator", operatorCfg, true, "present each cell or case", autonomousJudgeMode},
		}
		for _, row := range rows {
			t.Run(row.name, func(t *testing.T) {
				t.Parallel()
				in := judge(t)
				table, err := JudgeTable(in, row.cfg, seatRegistry(t))
				if err != nil {
					t.Fatalf("JudgeTable() = _, %v; want nil error", err)
				}
				if err := table.Validate(stencilsDir, ""); err != nil {
					t.Fatalf("table.Validate() = %v; want nil", err)
				}
				lead := table.Chair()
				if len(table.Seats) != 1 || lead.Name != "" || table.RolePrefix != "drift-judge" || table.Segment != segmentcolor.Review || table.Timeout != judgeTimeout || table.Interactive != row.interactive {
					t.Errorf("table = %d seats, lead %q, role %q, segment %q, timeout %v, interactive %v; want the lone lead of role drift-judge on the review segment, interactive %v", len(table.Seats), lead.Name, table.RolePrefix, table.Segment, table.Timeout, table.Interactive, row.interactive)
				}
				if lead.Engine != "claude" || lead.Model != "opus" || lead.Effort != "high" {
					t.Errorf("lead engine/model/effort = %q/%q/%q; want claude/opus/high", lead.Engine, lead.Model, lead.Effort)
				}
				if !slices.Equal(lead.Outputs, []string{in.JudgementPath}) || !slices.Equal(lead.Skills, []string{"scribe:prose"}) {
					t.Errorf("lead outputs %v and skills %v; want judgement.yaml alone and scribe:prose", lead.Outputs, lead.Skills)
				}

				prompt := seatPrompt(t, stencilsDir, worktreeRoot, in.Slug, table)
				for _, want := range []string{in.RequestPath, in.JudgementPath, in.DecisionRecordPath, "the-parent", row.want} {
					if !strings.Contains(prompt, want) {
						t.Errorf("judge prompt lacks %q", want)
					}
				}
				if strings.Contains(prompt, row.absent) {
					t.Errorf("judge prompt holds %q", row.absent)
				}
			})
		}
	})
}
