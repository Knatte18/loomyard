// describe.go implements the Describe row's two halves.
// DescribeTable builds the seatengine.Table from the row's told values:
// the stencil, the model resolved from landing.yaml's describe key, and the one description file the session writes.
// NewDescriptionGate is the mechanical gate over that file; neither derives a path of its own.

package landingshed

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
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

// NewDescriptionGate returns the Describe row's gate closure:
// a shuttleengine.Gate that runs summaryparser.ValidateDescription over descriptionPath.
// A returned error passes through with the zero GateResult;
// no findings is a pass;
// findings are a failed result: Findings joins each Finding's Error() with `"; "`, and a logger.Warn logs them first.
// That Warn line is the only durable record of why a description was refused.
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

// DescribeInputs carries the told values one Describe session needs.
// Every field but RunRecordPath, PriorRunRecordPaths and DriftVerdict is required.
type DescribeInputs struct {
	// StencilsDir is the absolute directory the landing stencils are read from.
	StencilsDir string
	// DecisionRecordPath is the run's decision record, which the agent reads.
	DecisionRecordPath string
	// RunRecordPath is the live generation's webster summary.md, read only for manual-check items.
	// Optional: a run with no such record passes a path that does not exist, or an empty string.
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
	// MergeBase is the commit the task's diff is taken from:
	// the merge base of the task branch with the parent tip the task last merged.
	MergeBase string
	// Slug is the task's slug, the key for `lym board get`.
	Slug string
	// DriftVerdict is the task's regression check as a markdown section, which the landing appends to the description.
	// Optional: empty when no regression check ran.
	DriftVerdict string
}

// runRecordBullets renders one bullet per run record that exists on disk: every prior generation oldest first, then the live record.
// A path that does not exist is skipped, and the single line `  - (none)` stands in when no record exists.
// A stat error other than not-exist is returned.
func runRecordBullets(in DescribeInputs) (string, error) {
	paths := append(append([]string{}, in.PriorRunRecordPaths...), in.RunRecordPath)
	var lines []string
	for _, p := range paths {
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			return "", fmt.Errorf("stat run record %s: %w", p, err)
		}
		lines = append(lines, "  - `"+p+"`")
	}
	if len(lines) == 0 {
		return "  - (none)", nil
	}
	return strings.Join(lines, "\n"), nil
}

// DescribeTable builds the seatengine.Table of the Describe session: one lead that writes the change description.
// An empty input field other than RunRecordPath, PriorRunRecordPaths and DriftVerdict is an error naming the field.
// A stencil missing from in.StencilsDir is an error too.
// Both checks fail a mis-wired closure before the row runs, rather than rendering a blank path into the prompt.
// A run record that does not exist is left out of the prompt, and a stat error on one is returned.
// An empty DriftVerdict renders as a line saying no regression check ran.
// The table's values are the stencil's markers; `seatengine` renders the parent and edit directives from its geometry.
// The table carries no gate; the caller that wires the producer sets it.
// The `Segment` is the describe segment, because the session writes the pull request description.
func DescribeTable(in DescribeInputs, cfg Config, reg modelspec.Registry) (seatengine.Table, error) {
	required := []struct{ name, value string }{
		{"StencilsDir", in.StencilsDir},
		{"DecisionRecordPath", in.DecisionRecordPath},
		{"DescriptionPath", in.DescriptionPath},
		{"TaskBranch", in.TaskBranch},
		{"ParentBranch", in.ParentBranch},
		{"MergeBase", in.MergeBase},
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

	runRecords, err := runRecordBullets(in)
	if err != nil {
		return seatengine.Table{}, fmt.Errorf("landingshed: DescribeTable: %w", err)
	}

	verdict := in.DriftVerdict
	if verdict == "" {
		verdict = noDriftVerdict
	}
	table, err := describeSessionTable(describeStencilName, in.DescriptionPath, map[string]string{
		"drift_verdict":        verdict,
		"slug":                 in.Slug,
		"decision_record_path": in.DecisionRecordPath,
		"run_record_paths":     runRecords,
		"description_path":     in.DescriptionPath,
		"task_branch":          in.TaskBranch,
		"parent_branch":        in.ParentBranch,
		"merge_base":           in.MergeBase,
	}, cfg, reg)
	if err != nil {
		return seatengine.Table{}, fmt.Errorf("landingshed: DescribeTable: %w", err)
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
			Stencil:     stencilName,
			ModelChoice: shuttleengine.NewModelChoice(resolved),
			Outputs:     []string{descriptionPath},
			Skills:      describeSkills,
		}},
	}, nil
}
