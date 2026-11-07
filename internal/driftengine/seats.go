// seats.go — the expect agent's seat table, and the subject, told-value and model helpers it is built from.
//
// The expect agent is one lead seat whose one output is expected.yaml.
// The builders are pure: they stat nothing they were not told to read, and spawn nothing.

package driftengine

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// The roles of the retired Drift-Baseline and Drift-Verify agents, whose live chairs a Drift row ends on entry.
const (
	baselineRole = "drift-baseline"
	verifyRole   = "drift-verify"
)

// The expect agent's role and the stencil that opens it.
const (
	expectRole        = "drift-expect"
	expectStencilName = "drift-template-expect"
)

// expectTimeout is the expect agent's session timeout, since drift.yaml declares no timeout key.
const expectTimeout = 30 * time.Minute

// The subject kinds a task's statement is read from.
const (
	// SubjectDecisionRecord reads the task from its decision record.
	SubjectDecisionRecord = "decision-record"
	// SubjectBoardEntry reads the task from its board entry.
	SubjectBoardEntry = "board-entry"
)

// expectSkills are the skills the expect agent's spawn loads, in order.
var expectSkills = []string{"scribe:prose"}

// The expectation formats the expect stencil's `expectation_format` text names, one per diff mode.
const (
	linesExpectationFormat = "The listing holds `mode: lines`: for each case of the baseline, each line with its identity and text.\n" +
		"A keyed line carries its `key`, and its occurrence `n` when the key occurs more than once in the case; an unkeyed line carries `line`, its position among the case's unkeyed lines.\n" +
		"Write expected.yaml as a mapping with these keys, each optional:\n\n" +
		"```yaml\n" +
		"cases:\n" +
		"  <case name>:\n" +
		"    keys:\n" +
		"      - key: <key>\n" +
		"        n: <occurrence, only for a key that repeats>\n" +
		"        added: true   # only for a key the baseline lacks\n" +
		"        why: <why the value is expected to change>\n" +
		"    lines:\n" +
		"      - range: \"12\"   # or \"12-20\", or the open \"12-\"\n" +
		"        why: <why these lines are expected to change>\n" +
		"added:\n" +
		"  - case: <a case only the current capture is expected to hold>\n" +
		"    why: <why>\n" +
		"removed:\n" +
		"  - case: <a case only the baseline holds and the task removes>\n" +
		"    why: <why>\n" +
		"```\n\n" +
		"Every entry needs a non-empty `why`, and a case or key must be one the listing holds unless it is marked as added."
	commandExpectationFormat = "The listing holds `mode: command`: the case names of the baseline alone, since the declared compare command, not lym, compares the captures.\n" +
		"Write expected.yaml as a mapping with these keys, each optional:\n\n" +
		"```yaml\n" +
		"cases:\n" +
		"  <case name>:\n" +
		"    why: <why this case's compare is expected to differ>\n" +
		"added:\n" +
		"  - case: <a case only the current capture is expected to hold>\n" +
		"    why: <why>\n" +
		"removed:\n" +
		"  - case: <a case only the baseline holds and the task removes>\n" +
		"    why: <why>\n" +
		"```\n\n" +
		"A case holds one `why` and no `keys` or `lines`, and every entry needs a non-empty `why`."
)

// subjectFields are the told values the task's subject is read from.
type subjectFields struct {
	// Subject is SubjectDecisionRecord or SubjectBoardEntry.
	Subject string
	// Slug is the board entry's slug; required for SubjectBoardEntry.
	Slug string
	// DecisionRecordPath is the absolute path of the decision record, whose Added after Discussion entries amend the task.
	DecisionRecordPath string
}

// reading returns the sentence part that tells the agent where to read the task.
func (s subjectFields) reading() (string, error) {
	switch s.Subject {
	case SubjectDecisionRecord:
		return fmt.Sprintf("the decision record at `%s`, in which the `Added after Discussion` entries amend the task and win over the rest where they differ", s.DecisionRecordPath), nil
	case SubjectBoardEntry:
		if s.Slug == "" {
			return "", fmt.Errorf("Slug must not be empty for the %s subject", SubjectBoardEntry)
		}
		slug, err := json.Marshal(map[string]string{"slug": s.Slug})
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("the board entry for task `%s`, read with `lym board get '%s'`, where the `Added after Discussion` entries of the decision record at `%s` amend the entry and win over its body", s.Slug, slug, s.DecisionRecordPath), nil
	}
	return "", fmt.Errorf("Subject %q is neither %s nor %s", s.Subject, SubjectDecisionRecord, SubjectBoardEntry)
}

// requireTold returns the refusal naming the first empty field of fields, or nil.
func requireTold(builder string, fields []struct{ field, value string }) error {
	for _, f := range fields {
		if f.value == "" {
			return fmt.Errorf("drift: %s: %s must not be empty", builder, f.field)
		}
	}
	return nil
}

// resolveModel parses the model-spec and resolves it against reg into the seat's model choice.
func resolveModel(builder, spec string, reg modelspec.Registry) (shuttleengine.ModelChoice, error) {
	parsed, err := modelspec.Parse(spec)
	if err != nil {
		return shuttleengine.ModelChoice{}, fmt.Errorf("drift: %s: model-spec: %w", builder, err)
	}
	resolved, err := reg.Resolve(parsed)
	if err != nil {
		return shuttleengine.ModelChoice{}, fmt.Errorf("drift: %s: model-spec: %w", builder, err)
	}
	return shuttleengine.NewModelChoice(resolved), nil
}

// ExpectInputs are the told values one Drift-Expect agent spawn is composed from.
type ExpectInputs struct {
	// Subject is SubjectDecisionRecord or SubjectBoardEntry.
	Subject string
	// Slug is the board entry's slug; required for SubjectBoardEntry.
	Slug string
	// DecisionRecordPath is the absolute path of the task's decision record, which the prompt names for either subject.
	DecisionRecordPath string
	// PlanDir is the absolute path of the task's plan directory, and empty for a run that has no plan.
	PlanDir string
	// BaselineDir is the absolute path of the baseline capture's directory.
	BaselineDir string
	// ListingPath is the absolute path of the identity listing Drift-Expect wrote.
	ListingPath string
	// ExpectedPath is the absolute path the agent writes expected.yaml to.
	ExpectedPath string
	// DiffMode is DiffLines or DiffCommand, the mode the run compares under.
	DiffMode string
}

// ExpectTable builds the seatengine.Table of one Drift-Expect spawn.
// The spawn is one non-interactive lead whose only output is expected.yaml.
// An empty required field of in is refused by name, and cfg.ExpectModel is resolved against reg.
// The prompt names the subject, the plan when the run has one, the listing and the baseline.
// It also includes the expectation format of the diff mode.
// The table carries no gate; the producer sets it.
func ExpectTable(in ExpectInputs, cfg Config, reg modelspec.Registry) (seatengine.Table, error) {
	err := requireTold("ExpectTable", []struct{ field, value string }{
		{"DecisionRecordPath", in.DecisionRecordPath},
		{"BaselineDir", in.BaselineDir},
		{"ListingPath", in.ListingPath},
		{"ExpectedPath", in.ExpectedPath},
	})
	if err != nil {
		return seatengine.Table{}, err
	}
	var format string
	switch in.DiffMode {
	case DiffLines:
		format = linesExpectationFormat
	case DiffCommand:
		format = commandExpectationFormat
	default:
		return seatengine.Table{}, fmt.Errorf("drift: ExpectTable: DiffMode %q is neither %q nor %q", in.DiffMode, DiffLines, DiffCommand)
	}
	reading, err := subjectFields{in.Subject, in.Slug, in.DecisionRecordPath}.reading()
	if err != nil {
		return seatengine.Table{}, fmt.Errorf("drift: ExpectTable: %w", err)
	}
	choice, err := resolveModel("ExpectTable", cfg.ExpectModel, reg)
	if err != nil {
		return seatengine.Table{}, err
	}
	planReading := "none: this run has no plan, so the subject above is the whole statement of the change"
	if in.PlanDir != "" {
		planReading = fmt.Sprintf("the plan in `%s`, whose `00-overview.md` frames the change and whose card files name what each card changes", in.PlanDir)
	}

	return seatengine.Table{
		RolePrefix: expectRole,
		Segment:    segmentcolor.Plan,
		Timeout:    expectTimeout,
		Values: map[string]string{
			"subject_reading":    reading,
			"plan_reading":       planReading,
			"baseline_dir":       in.BaselineDir,
			"listing_path":       in.ListingPath,
			"expected_path":      in.ExpectedPath,
			"diff_mode":          in.DiffMode,
			"expectation_format": format,
		},
		Seats: []seatengine.Seat{{
			Stencil:     expectStencilName,
			ModelChoice: choice,
			Outputs:     []string{in.ExpectedPath},
			Skills:      append([]string(nil), expectSkills...),
		}},
	}, nil
}
