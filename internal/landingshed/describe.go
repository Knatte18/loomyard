// describe.go implements the Describe row's two halves.
// DescribeTable and DarnDescribeTable are the pure builders that turn the row's told values into a seatengine.Table: the stencil, the model resolved from landing.yaml's describe key, and the one description file the session writes.
// NewDescriptionGate is the mechanical gate over that file.
// Neither derives a path of its own (Told-Geometry Invariant).

package landingshed

import (
	"fmt"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
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

// darnDescribeStencilName is the registered name of the darn recipe's Describe prompt.
const darnDescribeStencilName = "landing-template-describe-darn"

// DarnDescribeInputs carries the told values one Describe session of a darn run needs. Every field is required.
type DarnDescribeInputs struct {
	// StencilsDir is the absolute directory the landing stencils are read from.
	StencilsDir string
	// ReviewDir is the review perch's run directory, read only for the findings of a round the review passed unconverged.
	ReviewDir string
	// DecisionRecordPath is the decision record the darn writer wrote, read beside the board entry for the scope and decisions the description covers.
	DecisionRecordPath string
	// DescriptionPath is the file the agent writes: the change description.
	DescriptionPath string
	// TaskBranch is the branch whose diff the description covers.
	TaskBranch string
	// ParentBranch is the branch the task lands on.
	ParentBranch string
	// Slug is the task's slug, the key for `lyx board get`.
	Slug string
}

// DescribeTable builds the seatengine.Table of the Describe session: one lead that writes the change description.
// An empty input field other than ParentName and PriorRunRecordPaths is an error naming the field, and so is a stencil missing from in.StencilsDir, so a mis-wired closure fails before the row runs rather than rendering a blank path into the prompt.
// The table's values are the stencil's markers; the seat engine renders the parent and edit directives from its geometry.
// The table carries no gate; the producer entry sets it.
// Segment is the describe segment, because the session writes the pull request description.
func DescribeTable(in DescribeInputs, cfg Config, reg modelspec.Registry) (seatengine.Table, error) {
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
			return seatengine.Table{}, fmt.Errorf("landingshed: DescribeTable: %s is empty", r.name)
		}
	}
	if _, err := stencilstore.Read(in.StencilsDir, describeStencilName); err != nil {
		return seatengine.Table{}, fmt.Errorf("landingshed: DescribeTable: %w", err)
	}

	table, err := describeSessionTable(describeStencilName, in.DescriptionPath, map[string]string{
		"slug":                 in.Slug,
		"decision_record_path": in.DecisionRecordPath,
		"run_record_paths":     runRecordBullets(in),
		"description_path":     in.DescriptionPath,
		"task_branch":          in.TaskBranch,
		"parent_branch":        in.ParentBranch,
	}, cfg, reg)
	if err != nil {
		return seatengine.Table{}, fmt.Errorf("landingshed: DescribeTable: %w", err)
	}

	return table, nil
}

// DarnDescribeTable builds the seatengine.Table of the Describe session of a darn run, which describes the diff from the board entry and the decision record, and still reads no run record.
// It is DescribeTable's twin in every other respect: the model and timeout come from cfg's describe keys, the role, segment and skills are the same, and an empty required field or a missing stencil is an error.
func DarnDescribeTable(in DarnDescribeInputs, cfg Config, reg modelspec.Registry) (seatengine.Table, error) {
	required := []struct{ name, value string }{
		{"StencilsDir", in.StencilsDir},
		{"ReviewDir", in.ReviewDir},
		{"DecisionRecordPath", in.DecisionRecordPath},
		{"DescriptionPath", in.DescriptionPath},
		{"TaskBranch", in.TaskBranch},
		{"ParentBranch", in.ParentBranch},
		{"Slug", in.Slug},
	}
	for _, r := range required {
		if r.value == "" {
			return seatengine.Table{}, fmt.Errorf("landingshed: DarnDescribeTable: %s is empty", r.name)
		}
	}
	if _, err := stencilstore.Read(in.StencilsDir, darnDescribeStencilName); err != nil {
		return seatengine.Table{}, fmt.Errorf("landingshed: DarnDescribeTable: %w", err)
	}

	table, err := describeSessionTable(darnDescribeStencilName, in.DescriptionPath, map[string]string{
		"slug":                 in.Slug,
		"review_dir":           in.ReviewDir,
		"decision_record_path": in.DecisionRecordPath,
		"description_path":     in.DescriptionPath,
		"task_branch":          in.TaskBranch,
		"parent_branch":        in.ParentBranch,
	}, cfg, reg)
	if err != nil {
		return seatengine.Table{}, fmt.Errorf("landingshed: DarnDescribeTable: %w", err)
	}

	return table, nil
}

// describeSessionTable builds a Describe session's table from the stencil registered as stencilName, with markers as the table's values.
// The one lead seat writes descriptionPath, its one output file, under cfg's describe model and timeout.
func describeSessionTable(stencilName, descriptionPath string, markers map[string]string, cfg Config, reg modelspec.Registry) (seatengine.Table, error) {
	parsed, err := modelspec.Parse(cfg.Describe)
	if err != nil {
		return seatengine.Table{}, fmt.Errorf("describe model-spec: %w", err)
	}
	resolved, err := reg.Resolve(parsed)
	if err != nil {
		return seatengine.Table{}, fmt.Errorf("describe model-spec: %w", err)
	}

	return seatengine.Table{
		RolePrefix: describeRole,
		Segment:    segmentcolor.Describe,
		Timeout:    time.Duration(cfg.DescribeTimeoutMin) * time.Minute,
		Values:     markers,
		Seats: []seatengine.Seat{{
			Stencil: stencilName,
			Model:   resolved.Model,
			Effort:  resolved.Params["effort"],
			Version: resolved.Params["version"],
			Outputs: []string{descriptionPath},
			Skills:  describeSkills,
		}},
	}, nil
}
