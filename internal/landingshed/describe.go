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

	"github.com/Knatte18/loomyard/internal/editdirective"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/parentdirective"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
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

// describeRole is the agent-name role this module's Describe spawn carries.
const describeRole = "describe"

// describeSkills are the skills the Describe session loads before its prompt.
var describeSkills = []string{"scribe:prose"}

// DescribeInputs carries the told values one Describe session needs. Every field is required.
type DescribeInputs struct {
	// StencilsDir is the absolute directory the landing stencils are read from.
	StencilsDir string
	// ParentName is the name of the session the Describe session escalates to.
	// Optional: empty renders the no-parent variant of the parent directive.
	ParentName string
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
// Segment is the describe segment, because the session writes the pull request description.
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

	spec, err := describeSessionSpec(describeStencilName, in.StencilsDir, in.ParentName, in.DescriptionPath, map[string]string{
		"slug":                 in.Slug,
		"decision_record_path": in.DecisionRecordPath,
		"run_record_paths":     runRecordBullets(in),
		"description_path":     in.DescriptionPath,
		"task_branch":          in.TaskBranch,
		"parent_branch":        in.ParentBranch,
	}, cfg, reg)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("landingshed: DescribeSpec: %w", err)
	}

	return spec, nil
}

// darnDescribeStencilName is the registered name of the darn recipe's Describe prompt.
const darnDescribeStencilName = "landing-template-describe-darn"

// DarnDescribeInputs carries the told values one Describe session of a darn run needs. Every field but ParentName is required.
type DarnDescribeInputs struct {
	// StencilsDir is the absolute directory the landing stencils are read from.
	StencilsDir string
	// ParentName is the name of the session the Describe session escalates to.
	// Optional: empty renders the no-parent variant of the parent directive.
	ParentName string
	// ReviewDir is the review perch's run directory, read only for the findings of a round the review passed unconverged.
	ReviewDir string
	// DescriptionPath is the file the agent writes: the change description.
	DescriptionPath string
	// TaskBranch is the branch whose diff the description covers.
	TaskBranch string
	// ParentBranch is the branch the task lands on.
	ParentBranch string
	// Slug is the task's slug, the key for `lyx board get`.
	Slug string
}

// DarnDescribeSpec builds the shuttleengine.Spec for the Describe session of a darn run, which describes the diff from the board entry with no decision record or run record to read.
// It is DescribeSpec's twin in every other respect: the model and timeout come from cfg's describe keys, the role, segment and skills are the same, and an empty required field is an error naming it.
func DarnDescribeSpec(in DarnDescribeInputs, cfg Config, reg modelspec.Registry) (shuttleengine.Spec, error) {
	required := []struct{ name, value string }{
		{"StencilsDir", in.StencilsDir},
		{"ReviewDir", in.ReviewDir},
		{"DescriptionPath", in.DescriptionPath},
		{"TaskBranch", in.TaskBranch},
		{"ParentBranch", in.ParentBranch},
		{"Slug", in.Slug},
	}
	for _, r := range required {
		if r.value == "" {
			return shuttleengine.Spec{}, fmt.Errorf("landingshed: DarnDescribeSpec: %s is empty", r.name)
		}
	}

	spec, err := describeSessionSpec(darnDescribeStencilName, in.StencilsDir, in.ParentName, in.DescriptionPath, map[string]string{
		"slug":             in.Slug,
		"review_dir":       in.ReviewDir,
		"description_path": in.DescriptionPath,
		"task_branch":      in.TaskBranch,
		"parent_branch":    in.ParentBranch,
	}, cfg, reg)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("landingshed: DarnDescribeSpec: %w", err)
	}

	return spec, nil
}

// describeSessionSpec builds a Describe session's Spec from the stencil registered as stencilName, filled with markers plus the parent and edit directives.
// The session writes descriptionPath, its one output file, under cfg's describe model and timeout.
func describeSessionSpec(stencilName, stencilsDir, parentName, descriptionPath string, markers map[string]string, cfg Config, reg modelspec.Registry) (shuttleengine.Spec, error) {
	parsed, err := modelspec.Parse(cfg.Describe)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("describe model-spec: %w", err)
	}
	resolved, err := reg.Resolve(parsed)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("describe model-spec: %w", err)
	}

	template, err := stencilstore.Read(stencilsDir, stencilName)
	if err != nil {
		return shuttleengine.Spec{}, err
	}
	directive, err := parentdirective.Directive(stencilsDir, parentName, false)
	if err != nil {
		return shuttleengine.Spec{}, err
	}
	editDirective, err := editdirective.Directive(stencilsDir)
	if err != nil {
		return shuttleengine.Spec{}, err
	}
	markers[parentdirective.MarkerName] = directive
	markers[editdirective.MarkerName] = editDirective
	prompt, err := stencil.Fill(template, markers)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("fill describe prompt: %w", err)
	}

	return shuttleengine.Spec{
		Prompt:      string(prompt),
		OutputFiles: []string{descriptionPath},
		Model:       resolved.Model,
		Effort:      resolved.Params["effort"],
		Version:     resolved.Params["version"],
		Interactive: false,
		Role:        describeRole,
		Segment:     segmentcolor.Describe,
		Skills:      describeSkills,
		Timeout:     time.Duration(cfg.DescribeTimeoutMin) * time.Minute,
	}, nil
}
