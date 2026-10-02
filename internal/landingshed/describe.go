// describe.go implements the Describe row's two halves: DescribeSpec, the pure composer that turns
// the row's told values into a shuttleengine.Spec (the stencil, the model resolved from
// landing.yaml's describe key, and the one description file the session writes), and
// NewDescriptionGate, the mechanical gate over that file. Neither derives a path of its own
// (Told-Geometry Invariant).

package landingshed

import (
	"fmt"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/summaryparser"
)

// NewDescriptionGate returns the Describe row's gate closure: a shuttleengine.Gate that runs
// summaryparser.ValidateDescription over descriptionPath, the same function
// `lyx loom validate-description` calls (Gate Self-Check Parity Invariant).
// A returned error passes through with the zero GateResult; no findings is a pass; findings are a
// failed result whose Findings joins each Finding's Error() with "; ", preceded by a logger.Warn
// because that line is the only durable record of why a description was refused.
func NewDescriptionGate(descriptionPath string) shuttleengine.Gate {
	return func() (shuttleengine.GateResult, error) {
		findings, err := summaryparser.ValidateDescription(descriptionPath)
		if err != nil {
			return shuttleengine.GateResult{}, err
		}
		if len(findings) == 0 {
			return shuttleengine.GateResult{Passed: true}, nil
		}

		parts := make([]string, len(findings))
		for i, f := range findings {
			parts[i] = f.Error()
		}
		formatted := strings.Join(parts, "; ")
		logger.Warn("landingshed: description gate failed validation", "gate", "Describe-Gate", "description", descriptionPath, "findings", formatted)
		return shuttleengine.GateResult{Passed: false, Findings: formatted}, nil
	}
}

// describeStencilName is the registered name of the Describe prompt.
const describeStencilName = "landing-template-describe"

// DescribeInputs carries the told values one Describe session needs. Every field is required.
type DescribeInputs struct {
	// StencilsDir is the absolute directory the landing stencils are read from.
	StencilsDir string
	// DecisionRecordPath is the run's decision record, which the agent reads.
	DecisionRecordPath string
	// RunRecordPath is the live generation's webster summary.md, read only for manual-check items.
	RunRecordPath string
	// PriorRunRecordPaths are the summary.md files of the generations a rework round retired, oldest first.
	// Optional: empty when no round has run.
	PriorRunRecordPaths []string
	// DescriptionPath is the file the agent writes: the change description.
	DescriptionPath string
	// TaskBranch is the branch whose diff the description covers.
	TaskBranch string
	// ParentBranch is the branch the task lands on.
	ParentBranch string
	// Slug is the task's slug, the key for `lyx board get`.
	Slug string
}

// runRecordBullets renders one bullet per run record: every prior generation oldest first, then the live record.
func runRecordBullets(in DescribeInputs) string {
	paths := append(append([]string{}, in.PriorRunRecordPaths...), in.RunRecordPath)
	lines := make([]string, len(paths))
	for i, p := range paths {
		lines[i] = "  - `" + p + "`"
	}
	return strings.Join(lines, "\n")
}

// DescribeSpec builds the shuttleengine.Spec for the Describe session. An empty input field is an
// error naming the field, so a mis-wired closure fails at spawn rather than rendering a blank path
// into the prompt. The prompt is read from in.StencilsDir via stencilstore.Read and filled with
// stencil.Fill, never composed from a Go string literal.
func DescribeSpec(in DescribeInputs, cfg Config, reg modelspec.Registry) (shuttleengine.Spec, error) {
	required := []struct{ name, value string }{
		{"StencilsDir", in.StencilsDir},
		{"DecisionRecordPath", in.DecisionRecordPath},
		{"RunRecordPath", in.RunRecordPath},
		{"DescriptionPath", in.DescriptionPath},
		{"TaskBranch", in.TaskBranch},
		{"ParentBranch", in.ParentBranch},
		{"Slug", in.Slug},
	}
	for _, r := range required {
		if r.value == "" {
			return shuttleengine.Spec{}, fmt.Errorf("landingshed: DescribeSpec: %s is empty", r.name)
		}
	}

	parsed, err := modelspec.Parse(cfg.Describe)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("landingshed: DescribeSpec: describe model-spec: %w", err)
	}
	resolved, err := reg.Resolve(parsed)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("landingshed: DescribeSpec: describe model-spec: %w", err)
	}

	template, err := stencilstore.Read(in.StencilsDir, describeStencilName)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("landingshed: DescribeSpec: %w", err)
	}
	prompt, err := stencil.Fill(template, map[string]string{
		"slug":                 in.Slug,
		"decision_record_path": in.DecisionRecordPath,
		"run_record_paths":     runRecordBullets(in),
		"description_path":     in.DescriptionPath,
		"task_branch":          in.TaskBranch,
		"parent_branch":        in.ParentBranch,
	})
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("landingshed: DescribeSpec: fill describe prompt: %w", err)
	}

	return shuttleengine.Spec{
		Prompt:      string(prompt),
		OutputFiles: []string{in.DescriptionPath},
		Model:       resolved.Model,
		Effort:      resolved.Params["effort"],
		Version:     resolved.Params["version"],
		Interactive: false,
		Role:        "describe",
		Timeout:     time.Duration(cfg.DescribeTimeoutMin) * time.Minute,
	}, nil
}
