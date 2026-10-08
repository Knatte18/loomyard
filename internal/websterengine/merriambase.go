// merriambase.go computes the start base the batchifier prices Merriam's session from: the line count of the texts Merriam loads at start, plus a fixed figure for the system prompt and tools.
// The base enters only where a partition is formed; a recorded partition keeps the estimates it was formed with.

package websterengine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/pattern"
)

// merriamFixedContext is the context, in tokens, of Merriam's system prompt, tools and the skills loaded at launch.
// tools/tokencount's Merriam fixed-context calibration section re-measures it from real runs; the value changes only when the operator updates it from that report.
// It is 20600, the measured Merriam session start, minus the computed size of the texts the base covers at the commit the constant was taken at:
// 392 lines (CLAUDE.md 59, the Master stencil 229 and its inlined PATTERN overview 104) at the template's context_per_line of 12, so the computed base reproduces the measured start over that tree.
const merriamFixedContext = 20600 - 392*12

// MerriamBaseOf returns the start base for the given texts: their summed line count, counted as batcher.DiskSizes counts a file's, and the fixed system-prompt-and-tools context.
func MerriamBaseOf(texts ...string) batcher.StartBase {
	lines := 0
	for _, text := range texts {
		lines += strings.Count(text, "\n")
		if text != "" && !strings.HasSuffix(text, "\n") {
			lines++
		}
	}
	return batcher.StartBase{Lines: lines, Fixed: merriamFixedContext}
}

// MerriamBase returns the start base of the Merriam session geom describes: CLAUDE.md and CLAUDE.local.md at the worktree root when present, the Master stencil with the orchestrator PATTERN directive, and the plan's 00-overview.md.
// An absent CLAUDE.md or CLAUDE.local.md adds nothing; any other failure is refused as transient, naming the file.
func MerriamBase(geom Geometry) (batcher.StartBase, error) {
	var texts []string
	for _, name := range []string{"CLAUDE.md", "CLAUDE.local.md"} {
		data, err := os.ReadFile(filepath.Join(geom.WorktreeRoot, name))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return batcher.StartBase{}, merriamBaseError(name, err)
		}
		texts = append(texts, string(data))
	}

	template, err := MasterTemplate(geom.StencilsDir)
	if err != nil {
		return batcher.StartBase{}, merriamBaseError("webster-template-master", err)
	}
	directive, err := pattern.Directive(geom.RepoRoot, geom.StencilsDir, pattern.RoleOrchestrator)
	if err != nil {
		return batcher.StartBase{}, merriamBaseError("PATTERN.md", err)
	}
	overview, err := os.ReadFile(filepath.Join(geom.PlanDir, planOverviewFile))
	if err != nil {
		return batcher.StartBase{}, merriamBaseError(planOverviewFile, err)
	}
	texts = append(texts, string(template), directive, string(overview))

	return MerriamBaseOf(texts...), nil
}

// merriamBaseError refuses a failure to read file for Merriam's start base as transient.
func merriamBaseError(file string, cause error) error {
	return fmt.Errorf("webster: compute Merriam's start base, reading %s: %w; way forward: transient, re-run the verb", file, cause)
}
