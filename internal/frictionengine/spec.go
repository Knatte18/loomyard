// spec.go implements buildReflectionSpec, the unexported builder that turns one reflection run's told
// values into a shuttleengine.Spec ready for the Shuttle seam, following mergeresolve.buildConflictSpec's
// own resolution shape.

package frictionengine

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// reflectionStencilName is the registered name of the reflection agent's prompt, named once here so
// it appears exactly once in this package's Go source.
const reflectionStencilName = "friction-template-reflection"

// buildReflectionSpec builds the shuttleengine.Spec for the reflection run: notes is the sorted set
// of note file names the scan found, and reportPath is the agent's mandatory output file.
//
// It follows mergeresolve.buildConflictSpec's exact resolution shape: modelspec.Parse(deps.FrictionSpec),
// then deps.Registry.Resolve(parsed), then Model/Effort/Version off the resolved value. The prompt is
// read from deps.StencilsDir via stencilstore.Read and filled with stencil.Fill — never composed from
// a Go string literal, per the Stencil Ownership Invariant.
//
// Interactive is false because lyx loom run is by definition the unattended path, and an interactive
// spec would hang waiting for a human who is not there. ForkSubagents is false because the reflection
// agent has nothing to fan out over, and authorizing forks with no user present is authorization for
// nothing.
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
	values := map[string]string{
		"friction_dir": deps.FrictionDir,
		"report_path":  reportPath,
		"note_list":    renderNoteList(notes),
	}
	prompt, err := stencil.Fill(template, values)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("frictionengine: buildReflectionSpec: fill reflection prompt: %w", err)
	}

	return shuttleengine.Spec{
		Prompt:        string(prompt),
		OutputFiles:   []string{reportPath},
		Model:         resolved.Model,
		Effort:        resolved.Params["effort"],
		Version:       resolved.Params["version"],
		Interactive:   false,
		ForkSubagents: false,
		Role:          "friction",
		Timeout:       deps.Timeout,
	}, nil
}

// renderNoteList renders notes as a markdown bullet list, one file name per line, in the order
// given.
func renderNoteList(notes []string) string {
	lines := make([]string, len(notes))
	for i, n := range notes {
		lines[i] = "- " + n
	}
	return strings.Join(lines, "\n")
}
