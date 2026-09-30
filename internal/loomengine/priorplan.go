// priorplan.go renders the prior-plan block the Plan-Write rotator appends to a respawned session's prompt.
// It reads the loom-template-prior-plan stencil from stencilsDir at call time and fills it via internal/stencil.

package loomengine

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// PriorPlanBlock renders the "loom-template-prior-plan" stencil from stencilsDir, naming archiveDir as the directory the earlier plan was moved into and listing movedFiles as one backtick-wrapped bullet per file.
// Both markers are required,
// so an empty archiveDir or an empty movedFiles fails with the unfilled-marker error.
// It writes nothing to disk and derives no path.
func PriorPlanBlock(stencilsDir, archiveDir string, movedFiles []string) (string, error) {
	template, err := stencilstore.Read(stencilsDir, "loom-template-prior-plan")
	if err != nil {
		return "", err
	}

	var list strings.Builder
	for i, name := range movedFiles {
		if i > 0 {
			list.WriteString("\n")
		}
		list.WriteString("- `" + name + "`")
	}

	rendered, err := stencil.Fill(template, map[string]string{
		"archive_dir": archiveDir,
		"moved_files": list.String(),
	})
	if err != nil {
		return "", fmt.Errorf("loom: render prior-plan block: %w", err)
	}
	return string(rendered), nil
}
