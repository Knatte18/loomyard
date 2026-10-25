// fixer.go builds the merge fixer's session table, the one cold session Parent-Align and Finalize spawn to fix a merge or test failure inside their row.
// FixerTable is the pure builder that turns the row's told values into a seatengine.Table; it derives no path of its own (Told-Geometry Invariant).

package landingshed

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// FixerMode names which row the merge fixer serves and so what it may change.
type FixerMode string

const (
	// FixerModeParentAlign is the fixer of Parent-Align: it also judges what the parent brought in against the decision record.
	FixerModeParentAlign FixerMode = "parent-align"
	// FixerModeFinalize is the fixer of Finalize: it fixes failing tests only.
	FixerModeFinalize FixerMode = "finalize"
)

// fixerStencilName is the registered name of the merge fixer's prompt.
const fixerStencilName = "landing-template-fixer"

// fixerRole is the agent-name role the merge fixer's spawn carries.
const fixerRole = "fixer"

// FixerInputs carries the told values one merge fixer session needs.
// Every field is required except PlanDir, FailureRecordPath and LanguageSkills; AlignedTip is required in parent-align mode only.
type FixerInputs struct {
	// StencilsDir is the absolute directory the landing stencils are read from.
	StencilsDir string
	// Mode is the row the fixer serves.
	Mode FixerMode
	// DecisionRecordPath is the run's decision record, which the agent reads.
	DecisionRecordPath string
	// MergeRecordPath is the merge record the agent reads.
	MergeRecordPath string
	// PlanDir is the run's plan directory; empty for a darn run, which has none.
	PlanDir string
	// FailureRecordPath is the landing failure record of the gate that failed; empty before any gate has failed.
	FailureRecordPath string
	// AlignedTip is the tip the parent's changes are read from: the merge record's aligned tip or, when it holds none, the entry's fork point.
	// Empty in finalize mode, where the fixer reads no parent change.
	AlignedTip string
	// MergedTip is the tip the merge left on the task branch.
	MergedTip string
	// ReportPath is the file the agent writes: its report.
	ReportPath string
	// TaskBranch is the branch the agent fixes and commits on.
	TaskBranch string
	// ParentBranch is the branch the task lands on.
	ParentBranch string
	// Slug is the task's slug.
	Slug string
	// LanguageSkills are the run's language skills the session loads before its prompt.
	LanguageSkills []string
}

// FixerTable builds the seatengine.Table of the merge fixer session: one lead that fixes on the task branch and writes the report at in.ReportPath.
// An empty required input field is an error naming the field, and so is an unknown mode and a stencil missing from in.StencilsDir, so a mis-wired closure fails before the row runs rather than rendering a blank path into the prompt.
// PlanDir and FailureRecordPath are optional values in both modes, and AlignedTip is optional in finalize mode only, so an empty AlignedTip in parent-align mode is refused by name.
// The table's timeout is one turn's, cfg.FixerTimeoutMin; the caller sets the gate and stretches the timeout over the turns it may spend.
func FixerTable(in FixerInputs, cfg Config, reg modelspec.Registry) (seatengine.Table, error) {
	required := []struct{ name, value string }{
		{"StencilsDir", in.StencilsDir},
		{"Mode", string(in.Mode)},
		{"DecisionRecordPath", in.DecisionRecordPath},
		{"MergeRecordPath", in.MergeRecordPath},
		{"MergedTip", in.MergedTip},
		{"ReportPath", in.ReportPath},
		{"TaskBranch", in.TaskBranch},
		{"ParentBranch", in.ParentBranch},
		{"Slug", in.Slug},
	}
	for _, r := range required {
		if r.value == "" {
			return seatengine.Table{}, fmt.Errorf("landingshed: FixerTable: %s is empty", r.name)
		}
	}
	optional := []string{"plan_dir", "failure_record_path"}
	switch in.Mode {
	case FixerModeParentAlign:
	case FixerModeFinalize:
		optional = append(optional, "aligned_tip")
	default:
		return seatengine.Table{}, fmt.Errorf("landingshed: FixerTable: unknown Mode %q", in.Mode)
	}
	if _, err := stencilstore.Read(in.StencilsDir, fixerStencilName); err != nil {
		return seatengine.Table{}, fmt.Errorf("landingshed: FixerTable: %w", err)
	}

	parsed, err := modelspec.Parse(cfg.Fixer)
	if err != nil {
		return seatengine.Table{}, fmt.Errorf("landingshed: FixerTable: fixer model-spec: %w", err)
	}
	resolved, err := reg.Resolve(parsed)
	if err != nil {
		return seatengine.Table{}, fmt.Errorf("landingshed: FixerTable: fixer model-spec: %w", err)
	}

	return seatengine.Table{
		RolePrefix: fixerRole,
		Segment:    segmentcolor.Landing,
		Timeout:    time.Duration(cfg.FixerTimeoutMin) * time.Minute,
		Values: map[string]string{
			"slug":                 in.Slug,
			"mode":                 string(in.Mode),
			"decision_record_path": in.DecisionRecordPath,
			"merge_record_path":    in.MergeRecordPath,
			"plan_dir":             in.PlanDir,
			"failure_record_path":  in.FailureRecordPath,
			"aligned_tip":          in.AlignedTip,
			"merged_tip":           in.MergedTip,
			"report_path":          in.ReportPath,
			"task_branch":          in.TaskBranch,
			"parent_branch":        in.ParentBranch,
		},
		Optional: optional,
		Seats: []seatengine.Seat{{
			Stencil:     fixerStencilName,
			ModelChoice: shuttleengine.NewModelChoice(resolved),
			Outputs:     []string{in.ReportPath},
			Skills:      in.LanguageSkills,
		}},
	}, nil
}
