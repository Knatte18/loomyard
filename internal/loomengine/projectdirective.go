// projectdirective.go renders the project record directive, an optional prompt section.
// The section binds a beam member's Discussion-Write lead and darn writer to the project records above its pair.

package loomengine

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// ProjectDirectiveMarker is the optional marker of the Discussion-Write and darn writer prompts;
// the project record directive fills it.
const ProjectDirectiveMarker = "project_directive"

// projectDirectiveStencil names the stencil that holds the project record directive's text.
const projectDirectiveStencil = "loom-directive-project-record"

// projectRecordsToken is the literal token of the directive stencil that the record list replaces.
const projectRecordsToken = "{{.project_records}}"

// ProjectRecordDirective renders the project record directive that names the project records that bind a member's run.
// The records arrive nearest beam first, and each becomes one bullet of the directive.
// It returns empty for no records, so a run under no beam gets no section.
// It reads the stencil from stencilsDir at call time.
// The result fills a prompt's marker,
// so the token is substituted with a plain replace and the text never passes through stencil.Fill.
// It opens and ends with a newline, since the marker shares its line with the pattern directive's,
// so an empty value leaves the prompt's bytes unchanged.
func ProjectRecordDirective(stencilsDir string, records []string) (string, error) {
	if len(records) == 0 {
		return "", nil
	}
	content, err := stencilstore.Read(stencilsDir, projectDirectiveStencil)
	if err != nil {
		return "", fmt.Errorf("loom: project record directive: %w", err)
	}
	bullets := make([]string, len(records))
	for i, record := range records {
		bullets[i] = "- `" + record + "`"
	}
	stripped := stencil.StripLeadingComment(string(content))
	rendered := strings.ReplaceAll(stripped, projectRecordsToken, strings.Join(bullets, "\n"))
	// The marker shares a line with the one before it, so the section opens and closes on a newline of its own.
	return "\n" + strings.TrimSpace(rendered) + "\n", nil
}
