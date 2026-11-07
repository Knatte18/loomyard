// spec.go implements buildConflictSpec, which builds the spec for one conflict-resolution attempt.
// It turns that attempt's told values into a shuttleengine.Spec for the Shuttle seam.

package mergeresolve

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/editdirective"
	"github.com/Knatte18/loomyard/internal/languageconfig"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/parentdirective"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// conflictStencilName is the registered name of the conflict-resolution prompt.
const conflictStencilName = "landing-template-conflict"

// ConflictRole is the agent-name role this module's conflict-resolution spawn carries.
const ConflictRole = "conflict"

// conflictSkills are the skills the conflict-resolution session loads before its prompt.
var conflictSkills = []string{"scribe:prose", "scribe:code-quality", "scribe:testing"}

// reportNamePrefix is the filename prefix of every attempt's resolution report.
// The attempt number and ".md" follow it, and code that builds or matches a report path uses it.
const reportNamePrefix = "conflict-resolution-r"

// buildConflictSpec builds the shuttleengine.Spec for one conflict-resolution attempt.
// attempt is the number of the session: 1 for the first try, 2 for the retry.
// paths are the worktree-relative conflicted paths the session is told to resolve.
//
// The prompt comes from a stencil, never a Go string literal, per the Stencil Ownership Invariant.
//
// OutputFiles names exactly one fresh, absolute path: the resolution report at
// <ScratchDir>/<reportNamePrefix><attempt>.md.
// Three properties of that choice are load-bearing:
//
//  1. Absolute rather than relative, because a relative entry resolves against a worktree root.
//     On an anchored layout that root is not this scratch directory's parent,
//     so the report would land in the wrong directory.
//  2. Per-attempt (r1, r2), because the spec validator rejects an entry that already exists on disk.
//     A retry that reused the first attempt's path would fail before the session started.
//  3. Never the conflicted paths themselves: they exist on disk, so the validator would reject them,
//     and the pre-run archive step would archive the files that need resolving.
//
// buildConflictSpec also creates ScratchDir with os.MkdirAll before returning.
// It does so on this write path, because a told directory may not exist yet.
// Creating a told directory is legal under the Told-Geometry Invariant; deriving one is not.
//
// The spec runs in the landing segment, because the session resolves a merge while the run lands.
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

	reportPath := filepath.Join(deps.ScratchDir, fmt.Sprintf("%s%d.md", reportNamePrefix, attempt))

	template, err := stencilstore.Read(deps.StencilsDir, conflictStencilName)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("mergeresolve: buildConflictSpec: %w", err)
	}
	directive, err := parentdirective.Directive(deps.StencilsDir, deps.ParentName, false)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("mergeresolve: buildConflictSpec: %w", err)
	}
	editDirective, err := editdirective.Directive(deps.StencilsDir)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("mergeresolve: buildConflictSpec: %w", err)
	}
	values := map[string]string{
		"conflicted_paths":         renderConflictedPaths(paths),
		"report_path":              reportPath,
		parentdirective.MarkerName: directive,
		editdirective.MarkerName:   editDirective,
	}
	prompt, err := stencil.Fill(template, values)
	if err != nil {
		return shuttleengine.Spec{}, fmt.Errorf("mergeresolve: buildConflictSpec: fill conflict prompt: %w", err)
	}

	return shuttleengine.Spec{
		Prompt:      string(prompt),
		OutputFiles: []string{reportPath},
		ModelChoice: shuttleengine.NewModelChoice(resolved),
		Interactive: false,
		Role:        ConflictRole,
		Segment:     segmentcolor.Landing,
		Skills:      languageconfig.AppendSkills(conflictSkills, deps.LanguageSkills),
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
