// fixer.go builds the merge fixer's session table, the one cold session that fixes a merge or test failure in a row,
// and the one-seat table that every landing session's table is built on.
// FixerTable is the pure builder that turns the row's told values into a seatengine.Table;
// it derives no path of its own (Told-Geometry Invariant).

package landingshed

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/agentconfig"
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
// Every field is required except PlanDir, FailureRecordPath, FixReviewPath and LanguageSkills;
// AlignedTip is required in parent-align mode only.
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
	// FixReviewPath is the report of a fix review whose findings the fixer fixes; empty when no review rejected the change.
	// Finalize alone sets it.
	FixReviewPath string
	// AlignedTip is the tip the parent's changes are read from:
	// the merge record's aligned tip or, when it holds none, the entry's fork point.
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
	// Slug is the task's slug, passed to the prompt as the `slug` value.
	Slug string
	// LanguageSkills are the run's language skills the session loads after its own, before its prompt.
	LanguageSkills []string
}

// FixerTable builds the seatengine.Table of the merge fixer session:
// one lead that fixes on the task branch and writes the report at in.ReportPath.
// An empty required input field is an error naming the field, and so is an unknown mode.
// A stencil missing from in.StencilsDir is an error too.
// These checks fail a mis-wired closure before the row runs, rather than rendering a blank path into the prompt.
// PlanDir, FailureRecordPath and FixReviewPath are optional values in both modes.
// AlignedTip is optional in finalize mode only, so an empty AlignedTip in parent-align mode is refused by name.
// The table's timeout is one turn's, cfg.FixerTimeoutMin;
// the caller sets the gate and stretches the timeout over the turns it may spend.
func FixerTable(in FixerInputs, cfg Config, reg modelspec.Registry) (seatengine.Table, error) {
	if err := requireValues("FixerTable", [][2]string{
		{"StencilsDir", in.StencilsDir},
		{"Mode", string(in.Mode)},
		{"DecisionRecordPath", in.DecisionRecordPath},
		{"MergeRecordPath", in.MergeRecordPath},
		{"MergedTip", in.MergedTip},
		{"ReportPath", in.ReportPath},
		{"TaskBranch", in.TaskBranch},
		{"ParentBranch", in.ParentBranch},
		{"Slug", in.Slug},
	}); err != nil {
		return seatengine.Table{}, err
	}
	optional := []string{"plan_dir", "failure_record_path", "fix_review_path"}
	switch in.Mode {
	case FixerModeParentAlign:
	case FixerModeFinalize:
		optional = append(optional, "aligned_tip")
	default:
		return seatengine.Table{}, fmt.Errorf("landingshed: FixerTable: unknown Mode %q", in.Mode)
	}

	table, err := soloSession{
		stencilsDir: in.StencilsDir,
		stencil:     fixerStencilName,
		role:        fixerRole,
		segment:     segmentcolor.Landing,
		configRole:  agentconfig.RoleMergeFixer,
		modelKey:    "fixer",
		modelSpec:   cfg.Fixer,
		timeoutMin:  cfg.FixerTimeoutMin,
		output:      in.ReportPath,
		skills:      in.LanguageSkills,
		values: map[string]string{
			"slug":                 in.Slug,
			"mode":                 string(in.Mode),
			"decision_record_path": in.DecisionRecordPath,
			"merge_record_path":    in.MergeRecordPath,
			"plan_dir":             in.PlanDir,
			"failure_record_path":  in.FailureRecordPath,
			"fix_review_path":      in.FixReviewPath,
			"aligned_tip":          in.AlignedTip,
			"merged_tip":           in.MergedTip,
			"report_path":          in.ReportPath,
			"task_branch":          in.TaskBranch,
			"parent_branch":        in.ParentBranch,
		},
		optional: optional,
	}.table(reg)
	if err != nil {
		return seatengine.Table{}, fmt.Errorf("landingshed: FixerTable: %w", err)
	}
	return table, nil
}

// soloSession is what a landing session with one lead seat runs.
// It holds the stencil it renders and the values it fills, the model-spec and timeout it runs under,
// and the one file it writes.
type soloSession struct {
	// stencilsDir is the absolute directory the stencil is read from.
	stencilsDir string
	stencil     string
	role        string
	segment     segmentcolor.Segment
	configRole  agentconfig.Role
	// modelKey names the landing.yaml key modelSpec comes from, which a model-spec error names.
	modelKey   string
	modelSpec  string
	timeoutMin int
	// output is the one file the seat writes.
	output   string
	skills   []string
	values   map[string]string
	optional []string
}

// table builds the seatengine.Table of s.
// A stencil missing from s.stencilsDir is an error, and so is a model-spec that does not parse or resolve against reg.
func (s soloSession) table(reg modelspec.Registry) (seatengine.Table, error) {
	if _, err := stencilstore.Read(s.stencilsDir, s.stencil); err != nil {
		return seatengine.Table{}, err
	}
	choice, err := resolveModelChoice(s.modelSpec, reg)
	if err != nil {
		return seatengine.Table{}, fmt.Errorf("%s model-spec: %w", s.modelKey, err)
	}
	return seatengine.Table{
		RolePrefix: s.role,
		Segment:    s.segment,
		Timeout:    time.Duration(s.timeoutMin) * time.Minute,
		Values:     s.values,
		Optional:   s.optional,
		Seats: []seatengine.Seat{{
			Stencil:     s.stencil,
			ModelChoice: choice,
			Outputs:     []string{s.output},
			ConfigRole:  s.configRole,
			Skills:      s.skills,
		}},
	}, nil
}

// resolveModelChoice parses the model-spec text spec and resolves it against reg into the choice a session runs on.
func resolveModelChoice(spec string, reg modelspec.Registry) (shuttleengine.ModelChoice, error) {
	parsed, err := modelspec.Parse(spec)
	if err != nil {
		return shuttleengine.ModelChoice{}, err
	}
	resolved, err := reg.Resolve(parsed)
	if err != nil {
		return shuttleengine.ModelChoice{}, err
	}
	return shuttleengine.NewModelChoice(resolved), nil
}

// requireValues returns an error naming the first of fields, each a name and its value, whose value is empty.
// The error opens with builder, the function whose inputs the fields are.
func requireValues(builder string, fields [][2]string) error {
	for _, field := range fields {
		if field[1] == "" {
			return fmt.Errorf("landingshed: %s: %s is empty", builder, field[0])
		}
	}
	return nil
}
