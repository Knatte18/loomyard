// spec.go builds the shuttleengine.Spec that runs the reflection agent over the friction notes.

package frictionengine

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/agentconfig"
	"github.com/Knatte18/loomyard/internal/editdirective"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/parentdirective"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// reflectionStencilName is the registered name of the reflection agent's prompt.
// It is spelled here once, so the package's Go source names the stencil in one place.
const reflectionStencilName = "friction-template-reflection"

// frictionRole is the agent-name role this module's reflection spawn carries.
const frictionRole = "friction"

// buildReflectionSpec builds the shuttleengine.Spec for the reflection run.
// notes is the sorted set of note file names the scan found, and reportPath is the agent's mandatory output file.
//
// The model is resolved in three steps:
// modelspec.Parse(deps.FrictionSpec), then deps.Registry.Resolve(parsed), then shuttleengine.NewModelChoice(resolved).
// The prompt is read from deps.StencilsDir with stencilstore.Read and filled with stencil.Fill.
// It is never composed from a Go string literal.
//
// Interactive is false because lym loom start is by definition the unattended path,
// and an interactive spec would hang waiting for a human who is not there.
// ForkSubagents is false because the reflection agent has nothing to fan out over.
// With no user present, authorizing forks grants nothing.
//
// Segment is the landing segment, because the reflection runs while the run lands.
func buildReflectionSpec(deps Deps, notes []string, reportPath string) (shuttleengine.Spec, error) {
	parsed, err := modelspec.Parse(deps.FrictionSpec)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("frictionengine: buildReflectionSpec: friction model-spec: %w", err)
	}
	resolved, err := deps.Registry.Resolve(parsed)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("frictionengine: buildReflectionSpec: friction model-spec: %w", err)
	}

	template, err := stencilstore.Read(deps.StencilsDir, reflectionStencilName)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("frictionengine: buildReflectionSpec: %w", err)
	}
	directive, err := parentdirective.Directive(deps.StencilsDir, deps.ParentName, false)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("frictionengine: buildReflectionSpec: %w", err)
	}
	editDirective, err := editdirective.Directive(deps.StencilsDir)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("frictionengine: buildReflectionSpec: %w", err)
	}
	values := map[string]string{
		"friction_dir":             deps.FrictionDir,
		"report_path":              reportPath,
		"note_list":                renderNoteList(notes),
		"task_slug":                deps.TaskSlug,
		parentdirective.MarkerName: directive,
		editdirective.MarkerName:   editDirective,
	}
	prompt, err := stencil.Fill(template, values)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("frictionengine: buildReflectionSpec: fill reflection prompt: %w", err)
	}

	return shuttleengine.Spec{
		Prompt:        string(prompt),
		OutputFiles:   []string{reportPath},
		ModelChoice:   shuttleengine.NewModelChoice(resolved),
		Interactive:   false,
		ForkSubagents: false,
		Role:          frictionRole,
		Segment:       segmentcolor.Landing,
		ConfigRole:    agentconfig.RoleFriction,
		Timeout:       deps.Timeout,
	}, nil
}

// renderNoteList renders notes as a markdown bullet list, one file name per line, in the order given.
func renderNoteList(notes []string) string {
	lines := make([]string, len(notes))
	for i, n := range notes {
		lines[i] = "- " + n
	}
	return strings.Join(lines, "\n")
}
