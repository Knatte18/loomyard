// snapshot.go renders the board entry snapshot a darn run commits beside its decision record.

package darnengine

import (
	"fmt"
	"strings"
)

// BoardEntrySnapshot renders a task's board entry as the markdown snapshot the darn run's Code-Review perch reads as its answer key.
// The snapshot keeps the entry's own text apart from the writer's decision record, so the perch can check the writer's additions against it.
// It carries the title as its heading, then the brief and the body, each left out when blank.
func BoardEntrySnapshot(slug, title, brief, body string) []byte {
	var out strings.Builder
	fmt.Fprintf(&out, "# Board entry `%s`: %s\n", slug, strings.TrimSpace(title))
	for _, part := range []string{brief, body} {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			fmt.Fprintf(&out, "\n%s\n", trimmed)
		}
	}
	return []byte(out.String())
}
