//go:build tier0

// seats_test.go — tier 0 tests for the two agents' seat tables, over a seeded stencils directory, a model registry of the built-in aliases and the prompts the seat engine composes without starting anything.

package driftengine

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/pattern"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
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

// TestSeatTables covers both builders: each missing required input refused by name, the lead's model, role, output, skills and timeout, the markers each prompt renders, the two subjects and an unparsable model.
func TestSeatTables(t *testing.T) {
	t.Parallel()

	cfg := Config{BaselineModel: "sonnet[effort=medium]", VerifyModel: "opus[effort=high]", VerifyMaxRuns: 5}

	baseline := func(t *testing.T) BaselineInputs {
		t.Helper()
		dir := t.TempDir()
		return BaselineInputs{
			StencilsDir:        stencilkit.Seed(t),
			WorktreeRoot:       t.TempDir(),
			Subject:            SubjectDecisionRecord,
			Slug:               "fix-the-thing",
			DecisionRecordPath: filepath.Join(dir, "decision-record.md"),
			BaselineReportPath: filepath.Join(dir, "baseline.md"),
		}
	}
	verify := func(t *testing.T) VerifyInputs {
		t.Helper()
		dir := t.TempDir()
		return VerifyInputs{
			StencilsDir:        stencilkit.Seed(t),
			WorktreeRoot:       t.TempDir(),
			Subject:            SubjectDecisionRecord,
			Slug:               "fix-the-thing",
			DecisionRecordPath: filepath.Join(dir, "decision-record.md"),
			ScopePath:          filepath.Join(dir, "scope.json"),
			BaselineReportPath: filepath.Join(dir, "baseline.md"),
			VerifyReportPath:   filepath.Join(dir, "verify-report.md"),
			CompareLog:         filepath.Join(dir, "compare.log"),
			BaselineHead:       "0123abc",
		}
	}

	t.Run("a missing required input is refused by name", func(t *testing.T) {
		t.Parallel()
		baselineRows := []struct {
			field string
			clear func(*BaselineInputs)
		}{
			{"StencilsDir", func(in *BaselineInputs) { in.StencilsDir = "" }},
			{"WorktreeRoot", func(in *BaselineInputs) { in.WorktreeRoot = "" }},
			{"DecisionRecordPath", func(in *BaselineInputs) { in.DecisionRecordPath = "" }},
			{"BaselineReportPath", func(in *BaselineInputs) { in.BaselineReportPath = "" }},
		}
		for _, row := range baselineRows {
			in := baseline(t)
			row.clear(&in)
			if _, err := BaselineTable(in, cfg, seatRegistry(t)); err == nil || !strings.Contains(err.Error(), row.field) {
				t.Errorf("BaselineTable() without %s = %v; want a refusal naming it", row.field, err)
			}
		}
		verifyRows := []struct {
			field string
			clear func(*VerifyInputs)
		}{
			{"StencilsDir", func(in *VerifyInputs) { in.StencilsDir = "" }},
			{"WorktreeRoot", func(in *VerifyInputs) { in.WorktreeRoot = "" }},
			{"DecisionRecordPath", func(in *VerifyInputs) { in.DecisionRecordPath = "" }},
			{"ScopePath", func(in *VerifyInputs) { in.ScopePath = "" }},
			{"BaselineReportPath", func(in *VerifyInputs) { in.BaselineReportPath = "" }},
			{"VerifyReportPath", func(in *VerifyInputs) { in.VerifyReportPath = "" }},
			{"CompareLog", func(in *VerifyInputs) { in.CompareLog = "" }},
			{"BaselineHead", func(in *VerifyInputs) { in.BaselineHead = "" }},
		}
		for _, row := range verifyRows {
			in := verify(t)
			row.clear(&in)
			if _, err := VerifyTable(in, cfg, seatRegistry(t)); err == nil || !strings.Contains(err.Error(), row.field) {
				t.Errorf("VerifyTable() without %s = %v; want a refusal naming it", row.field, err)
			}
		}
	})

	t.Run("the subject is refused unknown, and a board entry needs its slug", func(t *testing.T) {
		t.Parallel()
		in := baseline(t)
		in.Subject = "other"
		if _, err := BaselineTable(in, cfg, seatRegistry(t)); err == nil || !strings.Contains(err.Error(), `Subject "other"`) {
			t.Errorf("BaselineTable() with an unknown subject = %v; want a refusal naming it", err)
		}
		in = baseline(t)
		in.Subject, in.Slug = SubjectBoardEntry, ""
		if _, err := BaselineTable(in, cfg, seatRegistry(t)); err == nil || !strings.Contains(err.Error(), "Slug") {
			t.Errorf("BaselineTable() for a board entry without a slug = %v; want a refusal naming Slug", err)
		}
	})

	t.Run("an unparsable model is refused", func(t *testing.T) {
		t.Parallel()
		bad := Config{BaselineModel: "not a spec[", VerifyModel: "not a spec[", VerifyMaxRuns: 5}
		if _, err := BaselineTable(baseline(t), bad, seatRegistry(t)); err == nil || !strings.Contains(err.Error(), "model-spec") {
			t.Errorf("BaselineTable() with an unparsable model = %v; want a model-spec refusal", err)
		}
		if _, err := VerifyTable(verify(t), bad, seatRegistry(t)); err == nil || !strings.Contains(err.Error(), "model-spec") {
			t.Errorf("VerifyTable() with an unparsable model = %v; want a model-spec refusal", err)
		}
	})

	t.Run("the baseline lead and its prompt", func(t *testing.T) {
		t.Parallel()
		in := baseline(t)
		in.ScopeNote = "A scope is already recorded; write baseline.md alone against it."
		table, err := BaselineTable(in, cfg, seatRegistry(t))
		if err != nil {
			t.Fatalf("BaselineTable() = _, %v; want nil error", err)
		}
		if err := table.Validate(in.StencilsDir); err != nil {
			t.Fatalf("table.Validate() = %v; want nil", err)
		}
		lead := table.Chair()
		if len(table.Seats) != 1 || lead.Name != "" || table.RolePrefix != "drift-baseline" || table.Segment != segmentcolor.Plan || table.Timeout != baselineTimeout {
			t.Errorf("table = %d seats, lead %q, role %q, segment %q, timeout %v; want the lone lead of role drift-baseline on the plan segment", len(table.Seats), lead.Name, table.RolePrefix, table.Segment, table.Timeout)
		}
		if lead.Engine != "claude" || lead.Model != "sonnet" || lead.Effort != "medium" {
			t.Errorf("lead engine/model/effort = %q/%q/%q; want claude/sonnet/medium", lead.Engine, lead.Model, lead.Effort)
		}
		if !slices.Equal(lead.Outputs, []string{in.BaselineReportPath}) || !slices.Equal(lead.Skills, []string{"scribe:prose"}) {
			t.Errorf("lead outputs %v and skills %v; want baseline.md alone and scribe:prose", lead.Outputs, lead.Skills)
		}

		prompt := seatPrompt(t, in.StencilsDir, in.WorktreeRoot, in.Slug, table)
		for _, want := range []string{in.BaselineReportPath, in.DecisionRecordPath, in.ScopeNote, "the-parent"} {
			if !strings.Contains(prompt, want) {
				t.Errorf("baseline prompt lacks %q", want)
			}
		}

		entry := baseline(t)
		entry.Subject = SubjectBoardEntry
		entryTable, err := BaselineTable(entry, cfg, seatRegistry(t))
		if err != nil {
			t.Fatalf("BaselineTable() for a board entry = _, %v; want nil error", err)
		}
		entryPrompt := seatPrompt(t, entry.StencilsDir, entry.WorktreeRoot, entry.Slug, entryTable)
		if !strings.Contains(entryPrompt, `lym board get '{"slug":"fix-the-thing"}'`) {
			t.Errorf("board-entry prompt lacks the board reading")
		}
		if strings.Contains(entryPrompt, "the decision record at `"+entry.DecisionRecordPath+"`, in which") || strings.Contains(prompt, "lym board get") {
			t.Errorf("the two subjects render alike; want the decision-record and board-entry readings distinct")
		}
	})

	t.Run("the verify lead and its prompt", func(t *testing.T) {
		t.Parallel()
		in := verify(t)
		in.LanguageSkills = []string{"scribe:golang-testing", "scribe:testing"}
		const overview = "# PATTERN\n\n- `PATTERN-example` — an entry the verify agent must follow.\n"
		if err := os.WriteFile(pattern.File(in.WorktreeRoot), []byte(overview), 0o644); err != nil {
			t.Fatal(err)
		}
		table, err := VerifyTable(in, cfg, seatRegistry(t))
		if err != nil {
			t.Fatalf("VerifyTable() = _, %v; want nil error", err)
		}
		if err := table.Validate(in.StencilsDir); err != nil {
			t.Fatalf("table.Validate() = %v; want nil", err)
		}
		lead := table.Chair()
		if len(table.Seats) != 1 || lead.Name != "" || table.RolePrefix != "drift-verify" || table.Segment != segmentcolor.Review || table.Timeout != verifyTimeout {
			t.Errorf("table = %d seats, lead %q, role %q, segment %q, timeout %v; want the lone lead of role drift-verify on the review segment", len(table.Seats), lead.Name, table.RolePrefix, table.Segment, table.Timeout)
		}
		if lead.Engine != "claude" || lead.Model != "opus" || lead.Effort != "high" {
			t.Errorf("lead engine/model/effort = %q/%q/%q; want claude/opus/high", lead.Engine, lead.Model, lead.Effort)
		}
		wantSkills := []string{"scribe:prose", "scribe:code-quality", "scribe:testing", "scribe:golang-testing"}
		if !slices.Equal(lead.Outputs, []string{in.VerifyReportPath}) || !slices.Equal(lead.Skills, wantSkills) {
			t.Errorf("lead outputs %v and skills %v; want verify-report.md alone and %v", lead.Outputs, lead.Skills, wantSkills)
		}
		if baselineTimeout >= verifyTimeout {
			t.Errorf("baseline timeout %v is not shorter than the verify timeout %v", baselineTimeout, verifyTimeout)
		}
		if table.Values["pattern_directive"] == "" {
			t.Errorf("the verify table renders no PATTERN directive; want the implementer directive, since its agent changes code")
		}

		prompt := seatPrompt(t, in.StencilsDir, in.WorktreeRoot, in.Slug, table)
		wants := []string{"PATTERN-example", in.CompareLog, in.ScopePath, in.BaselineReportPath, in.VerifyReportPath, in.DecisionRecordPath, in.BaselineHead, "5 runs", "the-parent"}
		for _, want := range wants {
			if !strings.Contains(prompt, want) {
				t.Errorf("verify prompt lacks %q", want)
			}
		}
	})
}
