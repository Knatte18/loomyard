// spec.go implements DarnSpec, the darn writer's Spec factory, and its prompt composition.
// The writer is one non-interactive session fed to shuttle.Run, producing the change description as its one output file.
// DarnSpec is a pure composer: it stats nothing it was not told to read, and spawns nothing.

package darnengine

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/editdirective"
	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/parentdirective"
	"github.com/Knatte18/loomyard/internal/pattern"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
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
	// ParentName is the name of the run's parent session; empty renders the no-parent variant of the parent directive.
	ParentName string
	// Slug is the board entry's slug the writer reads its task from.
	Slug string
	// DescriptionPath is the absolute path the writer writes the change description to.
	DescriptionPath string
	// RejectionFindings is the reviewer's findings this spawn fixes; empty when the spawn answers no rejection.
	RejectionFindings string
	// PriorWork names what an earlier spawn left unfinished or broken; empty on a first spawn.
	PriorWork string
}

// DarnSpec builds the shuttleengine.Spec for one darn writer spawn.
// An empty required field of in is refused by name, and cfg.Writer is resolved against reg.
// The spec is non-interactive, carries the change description as its one output file, and runs for cfg.WriterTimeoutMin minutes.
// A friction directive that fails to render is logged and left empty, because the note is optional bookkeeping.
func DarnSpec(in DarnInputs, cfg Config, reg modelspec.Registry) (shuttleengine.Spec, error) {
	required := []struct{ field, value string }{
		{"StencilsDir", in.StencilsDir},
		{"SpecsDir", in.SpecsDir},
		{"WorktreeRoot", in.WorktreeRoot},
		{"Slug", in.Slug},
		{"DescriptionPath", in.DescriptionPath},
	}
	for _, r := range required {
		if r.value == "" {
			return shuttleengine.Spec{}, fmt.Errorf("darn: DarnSpec: %s must not be empty", r.field)
		}
	}

	parsed, err := modelspec.Parse(cfg.Writer)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("darn: DarnSpec: writer model-spec: %w", err)
	}
	resolved, err := reg.Resolve(parsed)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("darn: DarnSpec: writer model-spec: %w", err)
	}

	template, err := stencilstore.Read(in.StencilsDir, darnStencilName)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("darn: DarnSpec: %w", err)
	}

	patternDirective, err := pattern.Directive(in.WorktreeRoot, in.StencilsDir, pattern.RoleImplementer)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("darn: DarnSpec: %w", err)
	}
	parentDirective, err := parentdirective.Directive(in.StencilsDir, in.ParentName, false)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("darn: DarnSpec: parent directive: %w", err)
	}
	editDirective, err := editdirective.Directive(in.StencilsDir)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("darn: DarnSpec: %w", err)
	}
	frictionDirective, err := friction.Directive(friction.NotePath(in.FrictionDir, "Darn"), in.StencilsDir, friction.RoleImplementer)
	if err != nil {
		logger.Warn("darn: friction directive failed, continuing without one", "role", darnRole, "stencil", darnStencilName, "error", err)
		frictionDirective = ""
	}
	friction.WarnIfMarkerAbsent(template, darnStencilName, frictionDirective)

	prompt, err := stencil.FillOptional(template, map[string]string{
		"slug":                     in.Slug,
		"description_path":         in.DescriptionPath,
		"specs_dir":                in.SpecsDir,
		"rejection_findings":       orNone(in.RejectionFindings),
		"prior_work":               orNone(in.PriorWork),
		"pattern_directive":        patternDirective,
		friction.MarkerName:        frictionDirective,
		parentdirective.MarkerName: parentDirective,
		editdirective.MarkerName:   editDirective,
	}, []string{"pattern_directive", friction.MarkerName, parentdirective.MarkerName})
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("darn: compose writer prompt: %w", err)
	}

	return shuttleengine.Spec{
		Prompt:      string(prompt),
		OutputFiles: []string{in.DescriptionPath},
		Model:       resolved.Model,
		Effort:      resolved.Params["effort"],
		Version:     resolved.Params["version"],
		Interactive: false,
		Role:        darnRole,
		Segment:     segmentcolor.Darn,
		Skills:      darnSkills,
		Timeout:     time.Duration(cfg.WriterTimeoutMin) * time.Minute,
	}, nil
}

// orNone returns value, or noneToken when value is empty.
func orNone(value string) string {
	if value == "" {
		return noneToken
	}
	return value
}
