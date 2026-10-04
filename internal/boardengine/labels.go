// labels.go — label validation against the board's configured Vocabulary.
//
// validateLabels is the one check that an entry's labels are configured, unique and carry the type labels its kind needs.
// Every refusal names the entry's slug and board.yaml's lists as the way forward.

package boardengine

import (
	"fmt"
	"strings"
)

// validateLabels refuses an entry whose labels the vocabulary does not accept; label order is preserved as given.
func validateLabels(t Task, v Vocabulary) error {
	seen := make(map[string]bool, len(t.Labels))
	var typeLabels []string
	for _, label := range t.Labels {
		if !v.Known(label) {
			return fmt.Errorf("entry %q carries label %q, which is in neither the types nor the labels list of board.yaml: add it to one of them, or drop it from the entry", t.Slug, label)
		}
		if seen[label] {
			return fmt.Errorf("entry %q carries label %q more than once: list each label once", t.Slug, label)
		}
		seen[label] = true
		if v.IsType(label) {
			typeLabels = append(typeLabels, label)
		}
	}

	switch {
	case t.Kind == KindNote && len(typeLabels) != 1:
		carried := "none"
		if len(typeLabels) > 0 {
			carried = strings.Join(typeLabels, ", ")
		}
		return fmt.Errorf("note %q must carry exactly one type label from the types list of board.yaml, and carries: %s", t.Slug, carried)
	case t.Kind == KindTask && len(typeLabels) == 0:
		return fmt.Errorf("task %q must carry a type label from the types list of board.yaml: add one to its labels", t.Slug)
	}
	return nil
}
