// seats.go — the seat tables of the two drift agents, the baseline agent and the verify agent, and the told inputs they are built from.
//
// Each agent is one non-interactive lead seat whose one output is its report.
// The builders are pure: they stat nothing they were not told to read, and spawn nothing.

package driftengine

import (
	"encoding/json"
	"fmt"
	"time"

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
	// SubjectDecisionRecord reads the task from its decision record, as a loom run does.
	SubjectDecisionRecord = "decision-record"
	// SubjectBoardEntry reads the task from its board entry, as a darn run does.
	SubjectBoardEntry = "board-entry"
)

// baselineSkills and verifySkills are the skills the two agents' spawns load, in order.
var (
	baselineSkills = []string{"scribe:prose"}
	verifySkills   = []string{"scribe:prose", "scribe:code-quality", "scribe:testing"}
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
		return fmt.Sprintf("the board entry for task `%s`, read with `lyx board get '%s'`, where the `Added after Discussion` entries of the decision record at `%s` amend the entry and win over its body", s.Slug, slug, s.DecisionRecordPath), nil
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

// BaselineTable builds the seatengine.Table of one Drift-Baseline spawn: one lead whose only output is baseline.md.
// An empty required field of in is refused by name, and cfg.BaselineModel is resolved against reg.
// The table carries no gate and no scope record output, since Go writes the record and a fresh spawn archives its outputs.
func BaselineTable(in BaselineInputs, cfg Config, reg modelspec.Registry) (seatengine.Table, error) {
	err := requireTold("BaselineTable", []struct{ field, value string }{
		{"StencilsDir", in.StencilsDir},
		{"WorktreeRoot", in.WorktreeRoot},
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

	return seatengine.Table{
		RolePrefix: verifyRole,
		Segment:    segmentcolor.Review,
		Timeout:    verifyTimeout,
		Values: map[string]string{
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
