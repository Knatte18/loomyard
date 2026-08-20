// spec.go implements buildConflictSpec, the unexported builder that turns one conflict-resolution
// attempt's told values into a shuttleengine.Spec ready for the Shuttle seam, following
// loomengine.DiscussionSpec's own resolution shape.

package mergeresolve

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// conflictStencilName is the registered name of the conflict-resolution prompt (card 18/19).
const conflictStencilName = "landing-template-conflict"

// buildConflictSpec builds the shuttleengine.Spec for one conflict-resolution attempt: attempt
// numbers the session (1 for the first try, 2 for the retry), and paths are the worktree-relative
// conflicted paths the session is told to resolve.
//
// It follows loomengine.DiscussionSpec's exact resolution shape: modelspec.Parse(deps.ConflictSpec),
// then deps.Registry.Resolve(spec), then Model/Effort/Version off the resolved value. The prompt is
// read from the told stencils directory via stencilstore.Read and filled with stencil.Fill — never
// composed from a Go string literal, per the Stencil Ownership Invariant.
//
// OutputFiles names exactly one fresh, absolute path: the resolution report at
// <ScratchDir>/conflict-resolution-r<attempt>.md. Three properties of that choice are load-bearing:
//
//  1. Absolute rather than relative, because a relative entry is resolved against a worktree root
//     that is not this scratch directory's parent on an anchored layout, which would land the
//     report in the wrong directory.
//  2. Per-attempt (r1, r2), because the spec validator rejects an entry that already exists on disk,
//     so a retry reusing the first attempt's path would fail before the session even started.
//  3. Never the conflicted paths themselves — those exist on disk by definition, so the validator
//     would reject them outright, and the pre-run archive step would archive the very files needing
//     resolution.
//
// buildConflictSpec also creates ScratchDir with os.MkdirAll before returning, on this write path,
// since a told directory may not exist yet — creating a told directory is legal under the
// Told-Geometry Invariant, deriving one would not be.
func buildConflictSpec(deps Deps, paths []string, attempt int) (shuttleengine.Spec, error) {
	parsed, err := modelspec.Parse(deps.ConflictSpec)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("mergeresolve: buildConflictSpec: conflict model-spec: %w", err)
	}
	resolved, err := deps.Registry.Resolve(parsed)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("mergeresolve: buildConflictSpec: conflict model-spec: %w", err)
	}

	if err := os.MkdirAll(deps.ScratchDir, 0o755); err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("mergeresolve: buildConflictSpec: create scratch directory %s: %w", deps.ScratchDir, err)
	}

	reportPath := filepath.Join(deps.ScratchDir, fmt.Sprintf("conflict-resolution-r%d.md", attempt))

	template, err := stencilstore.Read(deps.StencilsDir, conflictStencilName)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("mergeresolve: buildConflictSpec: %w", err)
	}
	values := map[string]string{
		"conflicted_paths": renderConflictedPaths(paths),
		"report_path":      reportPath,
	}
	prompt, err := stencil.Fill(template, values)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("mergeresolve: buildConflictSpec: fill conflict prompt: %w", err)
	}

	return shuttleengine.Spec{
		Prompt:      string(prompt),
		OutputFiles: []string{reportPath},
		Model:       resolved.Model,
		Effort:      resolved.Params["effort"],
		Version:     resolved.Params["version"],
		Interactive: false,
		Role:        "conflict",
		Timeout:     deps.Timeout,
	}, nil
}

// renderConflictedPaths renders paths as a markdown bullet list, one path per line.
func renderConflictedPaths(paths []string) string {
	lines := make([]string, len(paths))
	for i, p := range paths {
		lines[i] = "- " + p
	}
	return strings.Join(lines, "\n")
}
