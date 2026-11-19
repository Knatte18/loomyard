// spec.go implements DarnTable, the darn writer's seat-table builder, and DarnInputs, the told values it is built from.
// The writer is one non-interactive lead seat, producing the change description as its one output file.
// DarnTable is a pure builder: it stats nothing it was not told to read, and spawns nothing.

package darnengine

import (
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/agentconfig"
	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/pattern"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// darnStencilName is the registered name of the darn writer's stencil.
const darnStencilName = "darn-template-write"

// makeChangeStencil and makeChangeGatedStencil are the fragments that fill the writer stencil's Step 4 list.
// The first serves a writer that runs the gate itself; the second serves one whose verify gate has a cold fixer.
const (
	makeChangeStencil      = "darn-fragment-make-change"
	makeChangeGatedStencil = "darn-fragment-make-change-gated"
)

// reportStencil is the passage that has the writer commit the task's report under the configured directory.
const reportStencil = "darn-fragment-report"

// darnRole is the agent-name role the darn writer's spawn carries.
const darnRole = "darn"

// noneToken is what the stencil renders for a prior-work value the spawn has nothing to tell for.
const noneToken = "(none)"

// DarnInputs are the told values one darn writer spawn is composed from.
type DarnInputs struct {
	// StencilsDir is the directory the writer's stencil and the directive stencils are read from.
	StencilsDir string
	// SpecsDir is the deployed-specs directory the stencil points the writer at for the change description's format.
	SpecsDir string
	// WorktreeRoot is the task worktree the PATTERN overview is read from.
	WorktreeRoot string
	// FrictionDir is the directory the writer's friction note goes in; empty turns the friction directive off.
	FrictionDir string
	// Slug is the board entry's slug the writer reads its task from.
	Slug string
	// DescriptionPath is the absolute path the writer writes the change description to.
	DescriptionPath string
	// DecisionRecordPath is the absolute path of the decision record the writer reads as its scope.
	DecisionRecordPath string
	// StopMarkerPath is the absolute path the writer creates a marker at when it stops instead of making the change.
	StopMarkerPath string
	// ConditionStopMarkerPath is the absolute path of the marker the writer creates when a task-named condition stops it.
	ConditionStopMarkerPath string
	// PriorWork names what an earlier spawn left unfinished or broken; empty on a first spawn.
	PriorWork string
	// LanguageSkills are the hub's language skills the writer loads after the darn role's agents.yaml skills.
	// Empty means none.
	LanguageSkills []string
	// SubagentModel is the model the writer on Claude gives the fresh subagents it hands work to.
	// A writer on Codex runs them on its own model.
	SubagentModel string
	// GateFixed reports that a cold fixer clears the writer's verify gate,
	// so the writer and its subagents run no gate, except the writer's single measuring run of one test class,
	// and the writer asks its parent for a reported failure's single run.
	GateFixed bool
	// MeasureCall is the gate call the writer runs to measure one test class, in a form the hub's gate config can run.
	// Only the Step 4 list of a writer whose verify gate has a cold fixer names it.
	MeasureCall string
}

// DarnTable builds the seatengine.Table of one darn writer spawn: one lead that writes the change description.
// An empty required field of in is refused by name, and cfg.Writer is resolved against reg.
// The lead is non-interactive, carries the change description as its one output file,
// and the table runs for cfg.WriterTimeoutMin minutes.
// The table's values are the stencil's markers,
// and the seat engine renders the parent and edit directives from its geometry.
// The table carries no gate; the producer entry sets it.
// With in.GateFixed, the prompt's Step 4 has the writer and its subagents run no gate.
// The one exception is the writer's single measuring run of one test class for a value only a test run shows.
// They act only on what the verify gate's fixer leaves.
// The writer asks its parent for the single run of the test a task's reported failure names.
// Otherwise the step has them run the gate after each commit.
// With cfg.ReportDir set, a passage after the step has the writer commit the task's report under that directory.
// A friction directive that fails to render is logged and left empty, because the note is optional bookkeeping.
func DarnTable(in DarnInputs, cfg Config, reg modelspec.Registry) (seatengine.Table, error) {
	required := []struct{ field, value string }{
		{"StencilsDir", in.StencilsDir},
		{"SpecsDir", in.SpecsDir},
		{"WorktreeRoot", in.WorktreeRoot},
		{"Slug", in.Slug},
		{"DescriptionPath", in.DescriptionPath},
		{"DecisionRecordPath", in.DecisionRecordPath},
		{"StopMarkerPath", in.StopMarkerPath},
		{"ConditionStopMarkerPath", in.ConditionStopMarkerPath},
		{"SubagentModel", in.SubagentModel},
		{"MeasureCall", in.MeasureCall},
	}
	for _, r := range required {
		if r.value == "" {
			return seatengine.Table{}, fmt.Errorf("darn: DarnTable: %s must not be empty", r.field)
		}
	}

	parsed, err := modelspec.Parse(cfg.Writer)
	if err != nil {
		return seatengine.Table{}, fmt.Errorf("darn: DarnTable: writer model-spec: %w", err)
	}
	resolved, err := reg.Resolve(parsed)
	if err != nil {
		return seatengine.Table{}, fmt.Errorf("darn: DarnTable: writer model-spec: %w", err)
	}

	patternDirective, err := pattern.Directive(in.WorktreeRoot, in.StencilsDir, pattern.RoleImplementer, nil)
	if err != nil {
		return seatengine.Table{}, fmt.Errorf("darn: DarnTable: %w", err)
	}
	frictionDirective, err := friction.Directive(friction.NotePath(in.FrictionDir, "Darn-Write"), in.StencilsDir, friction.RoleImplementer)
	if err != nil {
		logger.Warn("darn: friction directive failed, continuing without one", "role", darnRole, "stencil", darnStencilName, "error", err)
		frictionDirective = ""
	}
	engine := shuttleengine.NewModelChoice(resolved).Engine
	makeChange, err := makeChangeList(in.StencilsDir, engine, in.GateFixed, in.SubagentModel, in.MeasureCall)
	if err != nil {
		return seatengine.Table{}, fmt.Errorf("darn: DarnTable: Step 4 list: %w", err)
	}
	reportDirective, err := reportPassage(in.StencilsDir, engine, cfg.ReportDir, in.Slug)
	if err != nil {
		return seatengine.Table{}, fmt.Errorf("darn: DarnTable: report passage: %w", err)
	}

	return seatengine.Table{
		RolePrefix: darnRole,
		Segment:    segmentcolor.Darn,
		Timeout:    time.Duration(cfg.WriterTimeoutMin) * time.Minute,
		Values: map[string]string{
			"slug":                       in.Slug,
			"description_path":           in.DescriptionPath,
			"decision_record_path":       in.DecisionRecordPath,
			"stop_marker_path":           in.StopMarkerPath,
			"condition_stop_marker_path": in.ConditionStopMarkerPath,
			"specs_dir":                  in.SpecsDir,
			"prior_work":                 orNone(in.PriorWork),
			"make_change":                makeChange,
			"report_directive":           reportDirective,
			"pattern_directive":          patternDirective,
			friction.MarkerName:          frictionDirective,
		},
		Optional: []string{"pattern_directive", "project_directive", "report_directive", friction.MarkerName},
		Seats: []seatengine.Seat{{
			Stencil:     darnStencilName,
			ModelChoice: shuttleengine.NewModelChoice(resolved),
			Outputs:     []string{in.DescriptionPath},
			ConfigRole:  agentconfig.RoleDarn,
			Skills:      in.LanguageSkills,
		}},
	}, nil
}

// makeChangeList returns the writer prompt's Step 4 list, worded for engine, the engine the writer launches on.
// It reads the list at call time from stencilsDir and fills subagentModel and measureCall into it;
// only the Claude wording names that model, and only the gated fragment names the call.
// It uses the gated fragment when gateFixed is set, and the ungated one otherwise.
func makeChangeList(stencilsDir, engine string, gateFixed bool, subagentModel, measureCall string) (string, error) {
	name := makeChangeStencil
	if gateFixed {
		name = makeChangeGatedStencil
	}
	content, err := stencilstore.ReadForEngine(stencilsDir, engine, name)
	if err != nil {
		return "", err
	}
	filled, err := stencil.Fill(content, map[string]string{"subagent_model": subagentModel, "measure_call": measureCall})
	if err != nil {
		return "", fmt.Errorf("fill stencil %s: %w", name, err)
	}
	return strings.TrimSpace(string(filled)), nil
}

// reportPassage returns the passage that has the writer commit the task's report under reportDir, worded for engine.
// The passage leads with a blank line, so it follows the Step 4 list on that list's line,
// and it names `<reportDir>/<slug>.md` as the report's path when the task names none.
// An empty reportDir returns "".
func reportPassage(stencilsDir, engine, reportDir, slug string) (string, error) {
	if reportDir == "" {
		return "", nil
	}
	content, err := stencilstore.ReadForEngine(stencilsDir, engine, reportStencil)
	if err != nil {
		return "", err
	}
	filled, err := stencil.Fill(content, map[string]string{"report_dir": reportDir, "report_path": path.Join(reportDir, slug+".md")})
	if err != nil {
		return "", fmt.Errorf("fill stencil %s: %w", reportStencil, err)
	}
	return "\n\n" + strings.TrimSpace(string(filled)), nil
}

// orNone returns value, or noneToken when value is empty.
func orNone(value string) string {
	if value == "" {
		return noneToken
	}
	return value
}
