// describe_test.go — untagged Tier-1 unit tests for DescribeSpec. The stencil is a fixture written
// into a t.TempDir() stencils dir, so no shipped default is read.

package landingshed

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
)

const describeStencilFixture = "{{.parent_directive}}\nslug={{.slug}}\ndecision={{.decision_record_path}}\nrun={{.run_record_paths}}\n" +
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

func TestDescribeSpec_ComposesSpec(t *testing.T) {
	in := newDescribeInputs(t)
	cfg := Config{Describe: "claude:sonnet[effort=high]", DescribeTimeoutMin: 7}

	spec, err := DescribeSpec(in, cfg, modelspec.Registry{})
	if err != nil {
		t.Fatalf("DescribeSpec = _, %v; want nil error", err)
	}
	if spec.Model != "sonnet" {
		t.Errorf("spec.Model = %q; want %q", spec.Model, "sonnet")
	}
	if spec.Effort != "high" {
		t.Errorf("spec.Effort = %q; want %q", spec.Effort, "high")
	}
	if spec.Role != "describe" {
		t.Errorf("spec.Role = %q; want %q", spec.Role, "describe")
	}
	if spec.Interactive {
		t.Errorf("spec.Interactive = true; want false")
	}
	if spec.Timeout != 7*time.Minute {
		t.Errorf("spec.Timeout = %v; want 7m", spec.Timeout)
	}
	if len(spec.OutputFiles) != 1 || spec.OutputFiles[0] != in.DescriptionPath {
		t.Errorf("spec.OutputFiles = %v; want [%q]", spec.OutputFiles, in.DescriptionPath)
	}
	for _, v := range []string{in.DecisionRecordPath, in.RunRecordPath, in.DescriptionPath, in.TaskBranch, in.ParentBranch, in.Slug} {
		if !strings.Contains(spec.Prompt, v) {
			t.Errorf("prompt %q does not contain input value %q", spec.Prompt, v)
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
			in.ParentName = tt.parentName

			spec, err := DescribeSpec(in, Config{Describe: "claude:sonnet"}, modelspec.Registry{})
			if err != nil {
				t.Fatalf("DescribeSpec = _, %v; want nil error", err)
			}
			if !strings.Contains(spec.Prompt, tt.want) {
				t.Errorf("prompt %q does not contain %q", spec.Prompt, tt.want)
			}
			if tt.parentName == "" && strings.Contains(spec.Prompt, "Your parent is") {
				t.Errorf("prompt %q carries the parent variant; want the no-parent variant", spec.Prompt)
			}
			if got := strings.Join(spec.Skills, ","); got != "scribe:prose" {
				t.Errorf("spec.Skills = %q; want scribe:prose", got)
			}
		})
	}
}

func TestDescribeSpec_RunRecordsOldestFirstLiveLast(t *testing.T) {
	cfg := Config{Describe: "claude:sonnet[effort=high]", DescribeTimeoutMin: 7}

	in := newDescribeInputs(t)
	in.PriorRunRecordPaths = []string{"/x/round-1/webster/summary.md", "/x/round-2/webster/summary.md"}
	spec, err := DescribeSpec(in, cfg, modelspec.Registry{})
	if err != nil {
		t.Fatalf("DescribeSpec = _, %v; want nil error", err)
	}
	first := strings.Index(spec.Prompt, in.PriorRunRecordPaths[0])
	second := strings.Index(spec.Prompt, in.PriorRunRecordPaths[1])
	live := strings.Index(spec.Prompt, in.RunRecordPath)
	if first < 0 || second < first || live < second {
		t.Errorf("prompt %q must list prior records oldest first, then the live one (indexes %d, %d, %d)", spec.Prompt, first, second, live)
	}

	in = newDescribeInputs(t)
	spec, err = DescribeSpec(in, cfg, modelspec.Registry{})
	if err != nil {
		t.Fatalf("DescribeSpec = _, %v; want nil error", err)
	}
	if strings.Count(spec.Prompt, "/webster/") != 1 || !strings.Contains(spec.Prompt, in.RunRecordPath) {
		t.Errorf("prompt %q must list only the live record when none is told", spec.Prompt)
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
			_, err := DescribeSpec(in, Config{Describe: "claude:sonnet", DescribeTimeoutMin: 1}, modelspec.Registry{})
			if err == nil || !strings.Contains(err.Error(), name) {
				t.Errorf("DescribeSpec with empty %s = _, %v; want an error naming the field", name, err)
			}
		})
	}
}

func TestDescribeSpec_UnparsableModelSpecIsError(t *testing.T) {
	in := newDescribeInputs(t)
	_, err := DescribeSpec(in, Config{Describe: "", DescribeTimeoutMin: 1}, modelspec.Registry{})
	if err == nil || !strings.Contains(err.Error(), "describe model-spec") {
		t.Errorf("DescribeSpec with unparsable describe = _, %v; want an error naming the describe model-spec", err)
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
