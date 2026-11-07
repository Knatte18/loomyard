// spec.go implements DarnTable, the darn writer's seat-table builder, and DarnInputs, the told values it is built from.
// The writer is one non-interactive lead seat, producing the change description as its one output file.
// DarnTable is a pure builder: it stats nothing it was not told to read, and spawns nothing.

package darnengine

import (
	"fmt"
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
	// SubagentModel is the model name the writer passes to the Agent tool for the fresh subagents it hands work to.
	SubagentModel string
	// GateFixed reports that a cold fixer clears the writer's verify gate, so the writer and its subagents run no gate.
	GateFixed bool
}

// DarnTable builds the seatengine.Table of one darn writer spawn: one lead that writes the change description.
// An empty required field of in is refused by name, and cfg.Writer is resolved against reg.
// The lead is non-interactive, carries the change description as its one output file,
// and the table runs for cfg.WriterTimeoutMin minutes.
// The table's values are the stencil's markers,
// and the seat engine renders the parent and edit directives from its geometry.
// The table carries no gate; the producer entry sets it.
// With in.GateFixed, the prompt's Step 4 has the writer and its subagents run no gate;
// they act only on what the verify gate's fixer leaves.
// Otherwise the step has them run the gate after each commit.
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
	makeChange, err := makeChangeList(in.StencilsDir, in.GateFixed, in.SubagentModel)
	if err != nil {
		return seatengine.Table{}, fmt.Errorf("darn: DarnTable: Step 4 list: %w", err)
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
			"pattern_directive":          patternDirective,
			friction.MarkerName:          frictionDirective,
		},
		Optional: []string{"pattern_directive", "project_directive", friction.MarkerName},
		Seats: []seatengine.Seat{{
			Stencil:     darnStencilName,
			ModelChoice: shuttleengine.NewModelChoice(resolved),
			Outputs:     []string{in.DescriptionPath},
			ConfigRole:  agentconfig.RoleDarn,
			Skills:      in.LanguageSkills,
		}},
	}, nil
}

// makeChangeList returns the writer prompt's Step 4 list.
// It reads the list at call time from stencilsDir and fills subagentModel into it.
// It uses the gated fragment when gateFixed is set, and the ungated one otherwise.
func makeChangeList(stencilsDir string, gateFixed bool, subagentModel string) (string, error) {
	name := makeChangeStencil
	if gateFixed {
		name = makeChangeGatedStencil
	}
	content, err := stencilstore.Read(stencilsDir, name)
	if err != nil {
		return "", err
	}
	filled, err := stencil.Fill(content, map[string]string{"subagent_model": subagentModel})
	if err != nil {
		return "", fmt.Errorf("fill stencil %s: %w", name, err)
	}
	return strings.TrimSpace(string(filled)), nil
}

// orNone returns value, or noneToken when value is empty.
func orNone(value string) string {
	if value == "" {
		return noneToken
	}
	return value
}
