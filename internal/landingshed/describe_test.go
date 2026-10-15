// describe_test.go — untagged Tier-1 unit tests for DescribeTable and DarnDescribeTable.
// The stencil is a fixture written into a t.TempDir() stencils dir, so no shipped default is read;
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
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
)

const describeStencilFixture = "{{.parent_directive}}\n{{.edit_directive}}\nslug={{.slug}}\ndecision={{.decision_record_path}}\nrun={{.run_record_paths}}\n" +
	"out={{.description_path}}\ntask={{.task_branch}}\nparent={{.parent_branch}}\n"

func newDescribeInputs(t *testing.T) DescribeInputs {
	t.Helper()
	stencilsDir := t.TempDir()
	stencilkit.SeedInto(t, stencilsDir)
	landingDir := filepath.Join(stencilsDir, "landing")
	if err := os.WriteFile(filepath.Join(landingDir, "landing-template-describe.md"), []byte(describeStencilFixture), 0o644); err != nil {
		t.Fatalf("WriteFile(describe stencil) = %v; want nil", err)
	}
	return DescribeInputs{
		StencilsDir:        stencilsDir,
		DecisionRecordPath: "/x/decision-record.md",
		RunRecordPath:      "/x/webster/summary.md",
		DescriptionPath:    "/x/landing/summary.md",
		TaskBranch:         "task-branch",
		ParentBranch:       "parent-branch",
		Slug:               "my-slug",
	}
}

// describeLeadPrompt returns the prompt of table's lead composed under a geometry over stencilsDir naming parentName.
func describeLeadPrompt(t *testing.T, stencilsDir, parentName string, table seatengine.Table) string {
	t.Helper()
	prompts, err := seatengine.Prompts(seatengine.Geometry{
		WorktreeRoot: "/x/worktree",
		AnchorPath:   "/x/worktree",
		StencilsDir:  stencilsDir,
		ParentName:   parentName,
		Shortname:    "ab",
		Slug:         "my-slug",
	}, table)
	if err != nil {
		t.Fatalf("seatengine.Prompts() = _, %v; want nil error", err)
	}
	return prompts[""]
}

// requireDescribeLead asserts table is the Describe session's: the lead alone, in the describe role, segment and skills, writing wantOutput, with no version choice, non-interactive and without forks.
func requireDescribeLead(t *testing.T, table seatengine.Table, wantOutput string) {
	t.Helper()
	lead := table.Chair()
	if len(table.Seats) != 1 {
		t.Fatalf("table has %d seats; want the lead alone", len(table.Seats))
	}
	if role := seatengine.SeatRole(table.RolePrefix, lead.Name); lead.Name != "" || role != "describe" {
		t.Errorf("lead name %q and role %q; want no name and the role %q", lead.Name, role, "describe")
	}
	if lead.Version != "" {
		t.Errorf("lead version = %q; want none", lead.Version)
	}
	if want := []string{wantOutput}; !slices.Equal(lead.Outputs, want) {
		t.Errorf("lead outputs = %v; want %v", lead.Outputs, want)
	}
	if want := []string{"scribe:prose"}; !slices.Equal(lead.Skills, want) {
		t.Errorf("lead skills = %v; want %v", lead.Skills, want)
	}
	if table.Segment != segmentcolor.Describe || table.Interactive || lead.ForkSubagents {
		t.Errorf("table segment/interactive/fork = %q/%v/%v; want %q/false/false", table.Segment, table.Interactive, lead.ForkSubagents, segmentcolor.Describe)
	}
}

func TestDescribeSpec_ComposesSpec(t *testing.T) {
	in := newDescribeInputs(t)
	parentName := "ab:cd:webster"
	cfg := Config{Describe: "claude:sonnet[effort=high]", DescribeTimeoutMin: 7}

	table, err := DescribeTable(in, cfg, modelspec.Registry{})
	if err != nil {
		t.Fatalf("DescribeTable = _, %v; want nil error", err)
	}
	if err := table.Validate(in.StencilsDir); err != nil {
		t.Fatalf("table.Validate() = %v; want nil", err)
	}

	lead := table.Chair()
	if lead.Engine != "claude" || lead.Model != "sonnet" || lead.Effort != "high" || table.Timeout != 7*time.Minute {
		t.Errorf("lead engine/model/effort, table timeout = %q/%q/%q, %v; want claude/sonnet/high, 7m", lead.Engine, lead.Model, lead.Effort, table.Timeout)
	}
	requireDescribeLead(t, table, in.DescriptionPath)

	prompt := describeLeadPrompt(t, in.StencilsDir, parentName, table)
	for _, v := range []string{in.DecisionRecordPath, in.RunRecordPath, in.DescriptionPath, in.TaskBranch, in.ParentBranch, in.Slug} {
		if !strings.Contains(prompt, v) {
			t.Errorf("prompt %q does not contain input value %q", prompt, v)
		}
	}
}

func TestDescribeSpec_SkillsAndParentDirective(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		parentName string
		want       string
	}{
		{"with parent", "ab:cd:webster", "ab:cd:webster"},
		{"no parent", "", "No parent is recorded"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			in := newDescribeInputs(t)

			table, err := DescribeTable(in, Config{Describe: "claude:sonnet"}, modelspec.Registry{})
			if err != nil {
				t.Fatalf("DescribeTable = _, %v; want nil error", err)
			}
			prompt := describeLeadPrompt(t, in.StencilsDir, tt.parentName, table)
			if !strings.Contains(prompt, tt.want) {
				t.Errorf("prompt %q does not contain %q", prompt, tt.want)
			}
			if !strings.Contains(prompt, "Edit or Write") {
				t.Errorf("prompt %q does not contain the edit directive's \"Edit or Write\" sentence", prompt)
			}
			if tt.parentName == "" && strings.Contains(prompt, "Your parent is") {
				t.Errorf("prompt %q carries the parent variant; want the no-parent variant", prompt)
			}
			if got := strings.Join(table.Chair().Skills, ","); got != "scribe:prose" {
				t.Errorf("lead.Skills = %q; want scribe:prose", got)
			}
		})
	}
}

func TestDescribeSpec_RunRecordsOldestFirstLiveLast(t *testing.T) {
	cfg := Config{Describe: "claude:sonnet[effort=high]", DescribeTimeoutMin: 7}

	in := newDescribeInputs(t)
	in.PriorRunRecordPaths = []string{"/x/round-1/webster/summary.md", "/x/round-2/webster/summary.md"}
	table, err := DescribeTable(in, cfg, modelspec.Registry{})
	if err != nil {
		t.Fatalf("DescribeTable = _, %v; want nil error", err)
	}
	prompt := describeLeadPrompt(t, in.StencilsDir, "", table)
	first := strings.Index(prompt, in.PriorRunRecordPaths[0])
	second := strings.Index(prompt, in.PriorRunRecordPaths[1])
	live := strings.Index(prompt, in.RunRecordPath)
	if first < 0 || second < first || live < second {
		t.Errorf("prompt %q must list prior records oldest first, then the live one (indexes %d, %d, %d)", prompt, first, second, live)
	}

	in = newDescribeInputs(t)
	table, err = DescribeTable(in, cfg, modelspec.Registry{})
	if err != nil {
		t.Fatalf("DescribeTable = _, %v; want nil error", err)
	}
	prompt = describeLeadPrompt(t, in.StencilsDir, "", table)
	if strings.Count(prompt, "/webster/") != 1 || !strings.Contains(prompt, in.RunRecordPath) {
		t.Errorf("prompt %q must list only the live record when none is told", prompt)
	}
}

func TestDescribeSpec_EmptyFieldIsError(t *testing.T) {
	fields := map[string]func(*DescribeInputs){
		"StencilsDir":        func(in *DescribeInputs) { in.StencilsDir = "" },
		"DecisionRecordPath": func(in *DescribeInputs) { in.DecisionRecordPath = "" },
		"RunRecordPath":      func(in *DescribeInputs) { in.RunRecordPath = "" },
		"DescriptionPath":    func(in *DescribeInputs) { in.DescriptionPath = "" },
		"TaskBranch":         func(in *DescribeInputs) { in.TaskBranch = "" },
		"ParentBranch":       func(in *DescribeInputs) { in.ParentBranch = "" },
		"Slug":               func(in *DescribeInputs) { in.Slug = "" },
	}
	for name, blank := range fields {
		t.Run(name, func(t *testing.T) {
			in := newDescribeInputs(t)
			blank(&in)
			_, err := DescribeTable(in, Config{Describe: "claude:sonnet", DescribeTimeoutMin: 1}, modelspec.Registry{})
			if err == nil || !strings.Contains(err.Error(), name) {
				t.Errorf("DescribeTable with empty %s = _, %v; want an error naming the field", name, err)
			}
		})
	}

	t.Run("missing stencil", func(t *testing.T) {
		in := newDescribeInputs(t)
		stencilkit.Remove(t, in.StencilsDir, describeStencilName)
		_, err := DescribeTable(in, Config{Describe: "claude:sonnet", DescribeTimeoutMin: 1}, modelspec.Registry{})
		if err == nil || !strings.Contains(err.Error(), describeStencilName) {
			t.Errorf("DescribeTable with no describe stencil = _, %v; want an error naming %q", err, describeStencilName)
		}
	})
}

// TestDarnDescribeSpec pins the darn Describe table: it fills the darn stencil with every told value under the describe keys, and refuses an empty required field by name.
func TestDarnDescribeSpec(t *testing.T) {
	t.Parallel()
	const darnStencilFixture = "{{.parent_directive}}\n{{.edit_directive}}\nslug={{.slug}}\nreviews={{.review_dir}}\nrecord={{.decision_record_path}}\nout={{.description_path}}\ntask={{.task_branch}}\nparent={{.parent_branch}}\n"
	newInputs := func(t *testing.T) DarnDescribeInputs {
		t.Helper()
		stencilsDir := t.TempDir()
		stencilkit.SeedInto(t, stencilsDir)
		if err := os.WriteFile(filepath.Join(stencilsDir, "landing", "landing-template-describe-darn.md"), []byte(darnStencilFixture), 0o644); err != nil {
			t.Fatalf("WriteFile(darn describe stencil) = %v; want nil", err)
		}
		return DarnDescribeInputs{
			StencilsDir:        stencilsDir,
			ReviewDir:          "/x/reviews/darn",
			DecisionRecordPath: "/x/discussion/decision-record.md",
			DescriptionPath:    "/x/landing/summary.md",
			TaskBranch:         "task-branch",
			ParentBranch:       "parent-branch",
			Slug:               "my-slug",
		}
	}
	cfg := Config{Describe: "claude:sonnet[effort=high]", DescribeTimeoutMin: 7}

	t.Run("composes from the darn stencil", func(t *testing.T) {
		t.Parallel()
		in := newInputs(t)
		parentName := "ab:cd:webster"
		table, err := DarnDescribeTable(in, cfg, modelspec.Registry{})
		if err != nil {
			t.Fatalf("DarnDescribeTable = _, %v; want nil error", err)
		}
		if err := table.Validate(in.StencilsDir); err != nil {
			t.Fatalf("table.Validate() = %v; want nil", err)
		}
		lead := table.Chair()
		if lead.Model != "sonnet" || table.Timeout != 7*time.Minute {
			t.Errorf("lead model, table timeout = %q, %v; want sonnet, 7m", lead.Model, table.Timeout)
		}
		requireDescribeLead(t, table, in.DescriptionPath)

		prompt := describeLeadPrompt(t, in.StencilsDir, parentName, table)
		for _, v := range []string{"reviews=" + in.ReviewDir, "record=" + in.DecisionRecordPath, "out=" + in.DescriptionPath, "task=" + in.TaskBranch, "parent=" + in.ParentBranch, "slug=" + in.Slug, "Edit or Write"} {
			if !strings.Contains(prompt, v) {
				t.Errorf("prompt %q does not contain %q", prompt, v)
			}
		}
	})

	fields := map[string]func(*DarnDescribeInputs){
		"StencilsDir":        func(in *DarnDescribeInputs) { in.StencilsDir = "" },
		"ReviewDir":          func(in *DarnDescribeInputs) { in.ReviewDir = "" },
		"DecisionRecordPath": func(in *DarnDescribeInputs) { in.DecisionRecordPath = "" },
		"DescriptionPath":    func(in *DarnDescribeInputs) { in.DescriptionPath = "" },
		"TaskBranch":         func(in *DarnDescribeInputs) { in.TaskBranch = "" },
		"ParentBranch":       func(in *DarnDescribeInputs) { in.ParentBranch = "" },
		"Slug":               func(in *DarnDescribeInputs) { in.Slug = "" },
	}
	for name, blank := range fields {
		t.Run("empty "+name, func(t *testing.T) {
			t.Parallel()
			in := newInputs(t)
			blank(&in)
			if _, err := DarnDescribeTable(in, cfg, modelspec.Registry{}); err == nil || !strings.Contains(err.Error(), name) {
				t.Errorf("DarnDescribeTable with empty %s = _, %v; want an error naming the field", name, err)
			}
		})
	}
}

func TestDescribeSpec_UnparsableModelSpecIsError(t *testing.T) {
	in := newDescribeInputs(t)
	_, err := DescribeTable(in, Config{Describe: "", DescribeTimeoutMin: 1}, modelspec.Registry{})
	if err == nil || !strings.Contains(err.Error(), "describe model-spec") {
		t.Errorf("DescribeTable with unparsable describe = _, %v; want an error naming the describe model-spec", err)
	}
}

func TestNewDescriptionGate(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) = %v; want nil", p, err)
		}
		return p
	}

	tests := []struct {
		name       string
		path       string
		wantPassed bool
		wantInFind string
	}{
		{"valid", write("ok.md", "# A title\n\nA body.\n"), true, ""},
		{"co-author trailer", write("trailer.md", "# A title\n\nBody.\n\nCo-Authored-By: X <x@y.z>\n"), false, "co-authored-by-trailer"},
		{"missing file", filepath.Join(dir, "absent.md"), false, "missing-file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := NewDescriptionGate(tt.path)()
			if err != nil {
				t.Fatalf("gate error = %v; want nil", err)
			}
			if res.Passed != tt.wantPassed {
				t.Errorf("Passed = %v; want %v (findings %q)", res.Passed, tt.wantPassed, res.Findings)
			}
			if !strings.Contains(res.Findings, tt.wantInFind) {
				t.Errorf("Findings = %q; want it to contain %q", res.Findings, tt.wantInFind)
			}
		})
	}
}
