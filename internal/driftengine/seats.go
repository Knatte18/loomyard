// seats.go — the seat tables of the two drift agents, the baseline agent and the verify agent, and the told inputs they are built from.
//
// Each agent is one lead seat whose one output is its report, interactive for the verify seat when drift.yaml sets verify_interactive.
// The builders are pure: they stat nothing they were not told to read, and spawn nothing.

package driftengine

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/languageconfig"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/pattern"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// The agent roles and the stencils that open them.
const (
	baselineRole        = "drift-baseline"
	verifyRole          = "drift-verify"
	baselineStencilName = "drift-template-baseline"
	verifyStencilName   = "drift-template-verify"
)

// The session timeouts of the two agents, since drift.yaml declares no timeout key.
const (
	baselineTimeout = 30 * time.Minute
	verifyTimeout   = 120 * time.Minute
)

// The subject kinds a task's statement is read from.
const (
	// SubjectDecisionRecord reads the task from its decision record, as a tapestry run does.
	SubjectDecisionRecord = "decision-record"
	// SubjectBoardEntry reads the task from its board entry, as a darn run does.
	SubjectBoardEntry = "board-entry"
)

// baselineSkills and verifySkills are the skills the two agents' spawns load, in order.
var (
	baselineSkills = []string{"scribe:prose"}
	verifySkills   = []string{"scribe:prose", "scribe:code-quality", "scribe:testing"}
)

// The verify stencil's mode and judging text, autonomous and with the operator in the pane.
const (
	autonomousVerifyMode = "No operator is present, so make best-judgment calls and never block on a dialog."
	operatorVerifyMode   = "The operator is present at your pane and decides each drift: you present it, discuss it and record what the operator decides.\n" +
		"Ask in plain text in the pane and wait for the operator's typed reply; never call the `AskUserQuestion` tool, whose modal dialog the run cannot drive."
	autonomousVerifyJudging = "Accept a drifted test with `lym drift accept <test> --reason <text>`, giving the reason the drift is intended.\n" +
		acceptanceKeepsOutput +
		"The run's parent can withdraw an acceptance; a withdrawn test is refused to you, so fix its drift or report it in the report."
	operatorVerifyJudging = "Present the drift to the operator one test at a time: for each quantity that drifted, its baseline value, its current value and the size of the deviation, then the baseline report's expectation for the test and your own judgment with its reason.\n" +
		"Discuss it with the operator, who decides whether the test's drift is accepted or fixed, and go on to the next test only once the operator has decided.\n" +
		"Record each acceptance the operator decides with `lym drift accept <test> --reason <text>`, giving the operator's reason, and never accept a test the operator has not decided on.\n" +
		acceptanceKeepsOutput +
		"Each test the operator decides to fix, you fix in Step 3."
	// acceptanceKeepsOutput tells either verify agent how an acceptance names its test and what output it keeps.
	acceptanceKeepsOutput = "Name the test as its profile file under the baseline directory is named, with or without the file's extension; any other name is refused.\n" +
		"The acceptance captures the test's output first and keeps it as the accepted output, so accept a test only once its output is final: a later change to that output halts the row.\n"
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

// BaselineInputs are the told values one Drift-Baseline agent spawn is composed from.
type BaselineInputs struct {
	// StencilsDir is the directory the agent's stencil and the directive stencils are read from.
	StencilsDir string
	// WorktreeRoot is the task worktree the agent works in.
	WorktreeRoot string
	// AnchorDir is the worktree's anchored directory, where a capture with no dir runs.
	AnchorDir string
	// Subject is SubjectDecisionRecord or SubjectBoardEntry.
	Subject string
	// Slug is the board entry's slug; required for SubjectBoardEntry.
	Slug string
	// DecisionRecordPath is the absolute path of the task's decision record.
	DecisionRecordPath string
	// BaselineReportPath is the absolute path the agent writes baseline.md to.
	BaselineReportPath string
	// ScopeNote says that a scope is already recorded, so the agent writes baseline.md alone against it; empty when none is.
	ScopeNote string
}

// captureReading returns the sentence part naming the declared capture command and the directory it runs in, which the baseline agent judges the suite's reach from.
// The directory is shown relative to worktreeRoot, and whole when it lies outside it.
func captureReading(step CommandStep, worktreeRoot, anchorDir string) string {
	if step.Command == "" {
		return "not declared yet, so judge the suite's reach from the repository's test setup alone"
	}
	resolved := gateslot.ResolveCommandDir(worktreeRoot, anchorDir, step.Dir)
	rel, err := filepath.Rel(worktreeRoot, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Sprintf("`%s`, run in `%s`", step.Command, resolved)
	}
	return fmt.Sprintf("`%s`, run in `%s` relative to the worktree root", step.Command, rel)
}

// BaselineTable builds the seatengine.Table of one Drift-Baseline spawn: one lead whose only output is baseline.md.
// An empty required field of in is refused by name, and cfg.BaselineModel is resolved against reg.
// The prompt names cfg.Capture's command and the directory it runs in, so the agent can check whether the suite reaches the task's change.
// The table carries no gate and no scope record output, since Go writes the record and a fresh spawn archives its outputs.
func BaselineTable(in BaselineInputs, cfg Config, reg modelspec.Registry) (seatengine.Table, error) {
	err := requireTold("BaselineTable", []struct{ field, value string }{
		{"StencilsDir", in.StencilsDir},
		{"WorktreeRoot", in.WorktreeRoot},
		{"AnchorDir", in.AnchorDir},
		{"DecisionRecordPath", in.DecisionRecordPath},
		{"BaselineReportPath", in.BaselineReportPath},
	})
	if err != nil {
		return seatengine.Table{}, err
	}
	reading, err := subjectFields{in.Subject, in.Slug, in.DecisionRecordPath}.reading()
	if err != nil {
		return seatengine.Table{}, fmt.Errorf("drift: BaselineTable: %w", err)
	}
	choice, err := resolveModel("BaselineTable", cfg.BaselineModel, reg)
	if err != nil {
		return seatengine.Table{}, err
	}

	return seatengine.Table{
		RolePrefix: baselineRole,
		Segment:    segmentcolor.Plan,
		Timeout:    baselineTimeout,
		Values: map[string]string{
			"subject_reading": reading,
			"capture_command": captureReading(cfg.Capture, in.WorktreeRoot, in.AnchorDir),
			"baseline_path":   in.BaselineReportPath,
			"scope_note":      in.ScopeNote,
		},
		Optional: []string{"scope_note"},
		Seats: []seatengine.Seat{{
			Stencil:     baselineStencilName,
			ModelChoice: choice,
			Outputs:     []string{in.BaselineReportPath},
			Skills:      append([]string(nil), baselineSkills...),
		}},
	}, nil
}

// VerifyInputs are the told values one Drift-Verify agent spawn is composed from.
type VerifyInputs struct {
	// StencilsDir is the directory the agent's stencil and the directive stencils are read from.
	StencilsDir string
	// WorktreeRoot is the task worktree the agent works in and the PATTERN overview is read from.
	WorktreeRoot string
	// Subject is SubjectDecisionRecord or SubjectBoardEntry.
	Subject string
	// Slug is the board entry's slug; required for SubjectBoardEntry.
	Slug string
	// DecisionRecordPath is the absolute path of the task's decision record.
	DecisionRecordPath string
	// ScopePath is the absolute path of the scope record.
	ScopePath string
	// BaselineReportPath is the absolute path of baseline.md.
	BaselineReportPath string
	// VerifyReportPath is the absolute path the agent writes verify-report.md to.
	VerifyReportPath string
	// CompareLog is the absolute path of the compare's log.
	CompareLog string
	// BaselineHead is the commit the baseline was captured on.
	BaselineHead string
	// LanguageSkills are the hub's language skills the agent loads after its own; empty for none.
	LanguageSkills []string
}

// VerifyTable builds the seatengine.Table of one Drift-Verify spawn: one lead whose only output is verify-report.md.
// An empty required field of in is refused by name, and cfg.VerifyModel is resolved against reg.
// With cfg.VerifyInteractive the lead is interactive, and its prompt has it present the drift to the operator and record the operator's decisions instead of judging alone.
// The agent changes code, so the table also renders the PATTERN implementer directive.
// The table carries no gate; the producer sets it.
func VerifyTable(in VerifyInputs, cfg Config, reg modelspec.Registry) (seatengine.Table, error) {
	err := requireTold("VerifyTable", []struct{ field, value string }{
		{"StencilsDir", in.StencilsDir},
		{"WorktreeRoot", in.WorktreeRoot},
		{"DecisionRecordPath", in.DecisionRecordPath},
		{"ScopePath", in.ScopePath},
		{"BaselineReportPath", in.BaselineReportPath},
		{"VerifyReportPath", in.VerifyReportPath},
		{"CompareLog", in.CompareLog},
		{"BaselineHead", in.BaselineHead},
	})
	if err != nil {
		return seatengine.Table{}, err
	}
	reading, err := subjectFields{in.Subject, in.Slug, in.DecisionRecordPath}.reading()
	if err != nil {
		return seatengine.Table{}, fmt.Errorf("drift: VerifyTable: %w", err)
	}
	choice, err := resolveModel("VerifyTable", cfg.VerifyModel, reg)
	if err != nil {
		return seatengine.Table{}, err
	}
	patternDirective, err := pattern.Directive(in.WorktreeRoot, in.StencilsDir, pattern.RoleImplementer, nil)
	if err != nil {
		return seatengine.Table{}, fmt.Errorf("drift: VerifyTable: %w", err)
	}
	mode, judging := autonomousVerifyMode, autonomousVerifyJudging
	if cfg.VerifyInteractive {
		mode, judging = operatorVerifyMode, operatorVerifyJudging
	}

	return seatengine.Table{
		RolePrefix:  verifyRole,
		Segment:     segmentcolor.Review,
		Timeout:     verifyTimeout,
		Interactive: cfg.VerifyInteractive,
		Values: map[string]string{
			"mode_rules":        mode,
			"judging_rules":     judging,
			"subject_reading":   reading,
			"compare_log":       in.CompareLog,
			"scope_path":        in.ScopePath,
			"baseline_path":     in.BaselineReportPath,
			"baseline_head":     in.BaselineHead,
			"report_path":       in.VerifyReportPath,
			"max_runs":          fmt.Sprint(cfg.VerifyMaxRuns),
			"pattern_directive": patternDirective,
		},
		Optional: []string{"pattern_directive"},
		Seats: []seatengine.Seat{{
			Stencil:     verifyStencilName,
			ModelChoice: choice,
			Outputs:     []string{in.VerifyReportPath},
			Skills:      languageconfig.AppendSkills(verifySkills, in.LanguageSkills),
		}},
	}, nil
}
