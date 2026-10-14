// spec.go implements DarnTable, the darn writer's seat-table builder, and DarnInputs, the told values it is built from.
// The writer is one non-interactive lead seat, producing the change description as its one output file.
// DarnTable is a pure builder: it stats nothing it was not told to read, and spawns nothing.

package darnengine

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/pattern"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
)

// darnStencilName is the registered name of the darn writer's stencil.
const darnStencilName = "darn-template-write"

// darnRole is the agent-name role the darn writer's spawn carries.
const darnRole = "darn"

// noneToken is what the stencil renders for a rejection-findings or prior-work value the spawn has nothing to tell for.
const noneToken = "(none)"

// darnSkills are the skills the darn writer's spawn loads, in order.
var darnSkills = []string{"scribe:prose", "scribe:code-quality"}

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
	// RejectionFindings is the reviewer's findings this spawn fixes; empty when the spawn answers no rejection.
	RejectionFindings string
	// PriorWork names what an earlier spawn left unfinished or broken; empty on a first spawn.
	PriorWork string
}

// DarnTable builds the seatengine.Table of one darn writer spawn: one lead that writes the change description.
// An empty required field of in is refused by name, and cfg.Writer is resolved against reg.
// The lead is non-interactive, carries the change description as its one output file, and the table runs for cfg.WriterTimeoutMin minutes.
// The table's values are the stencil's markers, and the seat engine renders the parent and edit directives from its geometry.
// The table carries no gate; the producer entry sets it.
// A friction directive that fails to render is logged and left empty, because the note is optional bookkeeping.
func DarnTable(in DarnInputs, cfg Config, reg modelspec.Registry) (seatengine.Table, error) {
	required := []struct{ field, value string }{
		{"StencilsDir", in.StencilsDir},
		{"SpecsDir", in.SpecsDir},
		{"WorktreeRoot", in.WorktreeRoot},
		{"Slug", in.Slug},
		{"DescriptionPath", in.DescriptionPath},
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

	patternDirective, err := pattern.Directive(in.WorktreeRoot, in.StencilsDir, pattern.RoleImplementer)
	if err != nil {
		return seatengine.Table{}, fmt.Errorf("darn: DarnTable: %w", err)
	}
	frictionDirective, err := friction.Directive(friction.NotePath(in.FrictionDir, "Darn-Write"), in.StencilsDir, friction.RoleImplementer)
	if err != nil {
		logger.Warn("darn: friction directive failed, continuing without one", "role", darnRole, "stencil", darnStencilName, "error", err)
		frictionDirective = ""
	}

	return seatengine.Table{
		RolePrefix: darnRole,
		Segment:    segmentcolor.Darn,
		Timeout:    time.Duration(cfg.WriterTimeoutMin) * time.Minute,
		Values: map[string]string{
			"slug":               in.Slug,
			"description_path":   in.DescriptionPath,
			"specs_dir":          in.SpecsDir,
			"rejection_findings": orNone(in.RejectionFindings),
			"prior_work":         orNone(in.PriorWork),
			"pattern_directive":  patternDirective,
			friction.MarkerName:  frictionDirective,
		},
		Optional: []string{"pattern_directive", friction.MarkerName},
		Seats: []seatengine.Seat{{
			Stencil: darnStencilName,
			Model:   resolved.Model,
			Effort:  resolved.Params["effort"],
			Version: resolved.Params["version"],
			Outputs: []string{in.DescriptionPath},
			Skills:  darnSkills,
		}},
	}, nil
}

// orNone returns value, or noneToken when value is empty.
func orNone(value string) string {
	if value == "" {
		return noneToken
	}
	return value
}
