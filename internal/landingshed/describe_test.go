// describe_test.go — tier0 unit tests for DescribeTable.
// The stencil is a fixture written into a t.TempDir() stencils dir, so no shipped default is read;
// the lead's prompt is composed through seatengine.Prompts, which starts nothing.

package landingshed

import (
	"fmt"
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
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

const describeStencilFixture = "{{.parent_directive}}\n{{.edit_directive}}\nslug={{.slug}}\ndecision={{.decision_record_path}}\nrun={{.run_record_paths}}\n" +
	"out={{.description_path}}\ntask={{.task_branch}}\nparent={{.parent_branch}}\nbase={{.merge_base}}\n"

// writeRunRecord writes a run record file at <dir>/<name>/webster/summary.md and returns its path.
func writeRunRecord(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name, "webster", "summary.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte("# Summary\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
	}
	return path
}

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
		RunRecordPath:      writeRunRecord(t, t.TempDir(), "live"),
		DescriptionPath:    "/x/landing/summary.md",
		TaskBranch:         "task-branch",
		ParentBranch:       "parent-branch",
		MergeBase:          "merge-base-sha",
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
	tiergate.Require(t, 0)
	in := newDescribeInputs(t)
	parentName := "ab:cd:webster"
	cfg := Config{Describe: "claude:sonnet[effort=high]", DescribeTimeoutMin: 7}

	table, err := DescribeTable(in, cfg, modelspec.Registry{})
	if err != nil {
		t.Fatalf("DescribeTable = _, %v; want nil error", err)
	}
	if err := table.Validate(in.StencilsDir, ""); err != nil {
		t.Fatalf("table.Validate() = %v; want nil", err)
	}

	lead := table.Chair()
	if lead.Engine != "claude" || lead.Model != "sonnet" || lead.Effort != "high" || table.Timeout != 7*time.Minute {
		t.Errorf("lead engine/model/effort, table timeout = %q/%q/%q, %v; want claude/sonnet/high, 7m", lead.Engine, lead.Model, lead.Effort, table.Timeout)
	}
	requireDescribeLead(t, table, in.DescriptionPath)
	if len(lead.Requires) != 0 {
		t.Errorf("lead Requires = %v; want none, a lone seat declares nothing", lead.Requires)
	}

	prompt := describeLeadPrompt(t, in.StencilsDir, parentName, table)
	for _, v := range []string{in.DecisionRecordPath, in.RunRecordPath, in.DescriptionPath, in.TaskBranch, in.ParentBranch, in.MergeBase, in.Slug} {
		if !strings.Contains(prompt, v) {
			t.Errorf("prompt %q does not contain input value %q", prompt, v)
		}
	}
}

func TestDescribeSpec_SkillsAndParentDirective(t *testing.T) {
	tiergate.Require(t, 0)
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

// TestDescribeSpec_RunRecordsOldestFirstLiveLast asserts the prompt lists each run record that exists, prior generations oldest first and the live one last, and `(none)` when none exists.
func TestDescribeSpec_RunRecordsOldestFirstLiveLast(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name string
		// prior holds whether each told prior record exists, oldest first; live whether the told live record does.
		prior []bool
		live  bool
	}{
		{"prior records oldest first then the live one", []bool{true, true}, true},
		{"the live record alone when no prior is told", nil, true},
		{"none when neither a prior nor the live record exists", []bool{false}, false},
		{"the prior record alone when the live one does not exist", []bool{true}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			in := newDescribeInputs(t)
			var want []string
			told := func(name string, exists bool) string {
				if exists {
					path := writeRunRecord(t, dir, name)
					want = append(want, path)
					return path
				}
				return filepath.Join(dir, name, "webster", "summary.md")
			}
			in.PriorRunRecordPaths = nil
			for i, exists := range tt.prior {
				in.PriorRunRecordPaths = append(in.PriorRunRecordPaths, told(fmt.Sprintf("round-%d", i+1), exists))
			}
			in.RunRecordPath = told("live", tt.live)

			table, err := DescribeTable(in, Config{Describe: "claude:sonnet[effort=high]", DescribeTimeoutMin: 7}, modelspec.Registry{})
			if err != nil {
				t.Fatalf("DescribeTable = _, %v; want nil error", err)
			}
			prompt := describeLeadPrompt(t, in.StencilsDir, "", table)
			if got := strings.Count(prompt, "  - `"); got != len(want) {
				t.Errorf("prompt %q lists %d run records; want %d", prompt, got, len(want))
			}
			previous := -1
			for _, path := range want {
				index := strings.Index(prompt, path)
				if index < previous {
					t.Errorf("prompt %q lists %q out of order; want oldest first, the live one last", prompt, path)
				}
				previous = index
			}
			if hasNone := strings.Contains(prompt, "(none)"); hasNone != (len(want) == 0) {
				t.Errorf("prompt %q has (none) = %t; want %t", prompt, hasNone, len(want) == 0)
			}
		})
	}

	t.Run("a stat error other than not-exist is returned", func(t *testing.T) {
		t.Parallel()
		in := newDescribeInputs(t)
		in.RunRecordPath = "bad\x00path"
		_, err := DescribeTable(in, Config{Describe: "claude:sonnet", DescribeTimeoutMin: 1}, modelspec.Registry{})
		if err == nil || !strings.Contains(err.Error(), "stat run record") {
			t.Errorf("DescribeTable with an unstatable run record = _, %v; want a stat error", err)
		}
	})
}

func TestDescribeSpec_EmptyFieldIsError(t *testing.T) {
	tiergate.Require(t, 0)
	fields := map[string]func(*DescribeInputs){
		"StencilsDir":        func(in *DescribeInputs) { in.StencilsDir = "" },
		"DecisionRecordPath": func(in *DescribeInputs) { in.DecisionRecordPath = "" },
		"DescriptionPath":    func(in *DescribeInputs) { in.DescriptionPath = "" },
		"TaskBranch":         func(in *DescribeInputs) { in.TaskBranch = "" },
		"ParentBranch":       func(in *DescribeInputs) { in.ParentBranch = "" },
		"MergeBase":          func(in *DescribeInputs) { in.MergeBase = "" },
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

func TestDescribeSpec_UnparsableModelSpecIsError(t *testing.T) {
	tiergate.Require(t, 0)
	in := newDescribeInputs(t)
	_, err := DescribeTable(in, Config{Describe: "", DescribeTimeoutMin: 1}, modelspec.Registry{})
	if err == nil || !strings.Contains(err.Error(), "describe model-spec") {
		t.Errorf("DescribeTable with unparsable describe = _, %v; want an error naming the describe model-spec", err)
	}
}

func TestNewDescriptionGate(t *testing.T) {
	tiergate.Require(t, 0)
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
