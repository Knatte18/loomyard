// twilldirective.go implements Directive, the renderer a spawning module calls to read the twill directive stencil from a told directory and fill it with its host values.
// See doc.go for the package-level rationale.

package twilldirective

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// MarkerName is the template marker a host carries for the rendered lens review directive.
const MarkerName = "twill_directive"

// directiveStencil names the stencil Directive reads.
const directiveStencil = "twill-directive"

// Lens is one review lens: its name and the prose its fork is given.
type Lens struct {
	Name string
	Text string
}

// Values are the host-specific values Directive fills into the directive stencil's markers.
type Values struct {
	// ReviewedFiles is the phrase naming the files each fork reviews, rendered verbatim with the host's own backticks.
	ReviewedFiles string
	// SpawnPoint is the sentence that opens the procedure and says when the writer runs it, rendered verbatim.
	SpawnPoint string
	// RejectionRule is the sentence that says how the writer may reject a finding, rendered verbatim.
	RejectionRule string
	// SeverityRule is the phrase that tells each fork how to grade a finding, rendered verbatim after "each with".
	SeverityRule string
	// RubricPath is the rubric the writer reads before spawning.
	RubricPath string
	// Lenses are the lenses to spawn, one fork each.
	Lenses []Lens
	// FormatStep names the host step that tells the writer how to write the reviewed files.
	FormatStep string
	// SelfCheckStep names the host step that tells the writer to run the self-check.
	SelfCheckStep string
	// SelfCheckCommand is the command the writer runs as the self-check.
	SelfCheckCommand string
	// FinalSelfCheckCommand is the command the writer runs as the final self-check, after the lens review is recorded.
	FinalSelfCheckCommand string
	// DispositionPlace names where the writer records the lens review.
	DispositionPlace string
}

// Directive returns the lens review directive read from stencilsDir, filled with v, without its leading banner and trailing newlines.
// Every marker is required: an empty value, or an empty Lenses, fails the fill with an error naming the marker.
// Every error names the stencil.
func Directive(stencilsDir string, v Values) (string, error) {
	values := map[string]string{
		"reviewed_files":           v.ReviewedFiles,
		"spawn_point":              v.SpawnPoint,
		"rejection_rule":           v.RejectionRule,
		"severity_rule":            v.SeverityRule,
		"rubric_path":              v.RubricPath,
		"lens_list":                bulletList(v.Lenses, func(l Lens) string { return l.Name + ": " + strings.Join(strings.Fields(l.Text), " ") }),
		"format_step":              v.FormatStep,
		"self_check_step":          v.SelfCheckStep,
		"self_check_command":       v.SelfCheckCommand,
		"final_self_check_command": v.FinalSelfCheckCommand,
		"disposition_place":        v.DispositionPlace,
	}
	content, err := stencilstore.Read(stencilsDir, directiveStencil)
	if err != nil {
		return "", fmt.Errorf("twilldirective: read stencil %s: %w", directiveStencil, err)
	}
	filled, err := stencil.Fill(content, values)
	if err != nil {
		return "", fmt.Errorf("twilldirective: fill stencil %s: %w", directiveStencil, err)
	}
	return strings.TrimRight(string(filled), "\r\n"), nil
}

// bulletList renders one "- " bullet per item on its own line, or the empty string for no items.
func bulletList[T any](items []T, text func(T) string) string {
	lines := make([]string, len(items))
	for i, item := range items {
		lines[i] = "- " + text(item)
	}
	return strings.Join(lines, "\n")
}
