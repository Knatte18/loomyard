// twilldirective.go implements Directive and WriteScope, the renderers a spawning module calls to read the two twill stencils from a told directory and fill them with its host values.
// See doc.go for the package-level rationale.

package twilldirective

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

const (
	// MarkerName is the template marker a host carries for the rendered lens review directive.
	MarkerName = "twill_directive"
	// WriteScopeMarkerName is the template marker a host carries for the rendered write-scope paragraph.
	WriteScopeMarkerName = "twill_write_scope"
)

const (
	// directiveStencil names the stencil Directive reads.
	directiveStencil = "twill-directive"
	// writeScopeStencil names the stencil WriteScope reads.
	writeScopeStencil = "twill-write-scope"
)

// Lens is one review lens: its name and the prose its fork is given.
type Lens struct {
	Name string
	Text string
}

// Values are the host-specific values Directive fills into the directive stencil's markers.
type Values struct {
	// ReviewedFiles are the files each fork reviews, one path each.
	ReviewedFiles []string
	// ReadyMarker is the file the writer creates once the self-check passes and the forks wait for.
	ReadyMarker string
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
	// DispositionPlace names where the writer records the lens review.
	DispositionPlace string
}

// Directive returns the lens review directive read from stencilsDir, filled with v, without its leading banner and trailing newlines.
// Every marker is required: an empty value, or an empty ReviewedFiles or Lenses, fails the fill with an error naming the marker.
// Every error names the stencil.
func Directive(stencilsDir string, v Values) (string, error) {
	values := map[string]string{
		"reviewed_files":     bulletList(v.ReviewedFiles, func(path string) string { return "`" + path + "`" }),
		"ready_marker":       v.ReadyMarker,
		"rubric_path":        v.RubricPath,
		"lens_list":          bulletList(v.Lenses, func(l Lens) string { return l.Name + ": " + strings.Join(strings.Fields(l.Text), " ") }),
		"format_step":        v.FormatStep,
		"self_check_step":    v.SelfCheckStep,
		"self_check_command": v.SelfCheckCommand,
		"disposition_place":  v.DispositionPlace,
	}
	return render(stencilsDir, directiveStencil, values)
}

// WriteScope returns the write-scope paragraph read from stencilsDir, filled with readyMarker, without its leading banner and trailing newlines.
// An empty readyMarker fails the fill with an error naming the marker; every error names the stencil.
func WriteScope(stencilsDir, readyMarker string) (string, error) {
	return render(stencilsDir, writeScopeStencil, map[string]string{"ready_marker": readyMarker})
}

// render reads the named stencil from stencilsDir and fills it with values, every marker required.
func render(stencilsDir, name string, values map[string]string) (string, error) {
	content, err := stencilstore.Read(stencilsDir, name)
	if err != nil {
		return "", fmt.Errorf("twilldirective: read stencil %s: %w", name, err)
	}
	filled, err := stencil.Fill(content, values)
	if err != nil {
		return "", fmt.Errorf("twilldirective: fill stencil %s: %w", name, err)
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
