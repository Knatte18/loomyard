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
)

const describeStencilFixture = "slug={{.slug}}\ndecision={{.decision_record_path}}\nrun={{.run_record_path}}\n" +
	"out={{.description_path}}\ntask={{.task_branch}}\nparent={{.parent_branch}}\n"

func newDescribeInputs(t *testing.T) DescribeInputs {
	t.Helper()
	stencilsDir := t.TempDir()
	landingDir := filepath.Join(stencilsDir, "landing")
	if err := os.MkdirAll(landingDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", landingDir, err)
	}
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
	cfg := Config{Describe: "claude:sonnet", DescribeTimeoutMin: 7}

	spec, err := DescribeSpec(in, cfg, modelspec.Registry{})
	if err != nil {
		t.Fatalf("DescribeSpec = _, %v; want nil error", err)
	}
	if spec.Model == "" {
		t.Errorf("spec.Model = %q; want non-empty", spec.Model)
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
