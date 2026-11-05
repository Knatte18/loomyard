// labels.go — label validation against the board's configured Vocabulary.
//
// validateLabels is the one check that an entry's labels are configured, unique and carry the type labels its kind needs.
// Every refusal names the entry's slug and board.yaml's lists as the way forward.

package boardengine

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/shedrun"
)

// validateLabels refuses an entry whose labels the vocabulary does not accept; label order is preserved as given.
// It also refuses the sketch type label on a task whose recipe is not raddle and on a project.
// prior is the snapshot's entry of the same slug, nil for a new entry; that rule judges only a write that adds the label, changes the kind or changes the recipe,
// so a legacy task that already carries the label keeps taking writes that leave all three alone.
func validateLabels(t Task, prior *Task, v Vocabulary) error {
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
	case t.Kind == KindProject && len(typeLabels) == 0:
		return fmt.Errorf("project %q must carry a type label from the types list of board.yaml: add one to its labels", t.Slug)
	}
	return checkSketchLabel(t, prior)
}

// checkSketchLabel refuses a project, and a task whose recipe is not raddle, that carries the sketch type label, when the write adds the label, changes the entry's kind or changes its recipe.
// A promotion of a discussed note names the raddle promote as the way forward, and any other write names the upsert.
func checkSketchLabel(t Task, prior *Task) error {
	if (t.Kind != KindTask && t.Kind != KindProject) || !slices.Contains(t.Labels, SketchType) {
		return nil
	}
	if t.Kind == KindTask && t.Recipe == shedrun.RecipeRaddle {
		return nil
	}
	if prior != nil && slices.Contains(prior.Labels, SketchType) && prior.Kind == t.Kind && prior.Recipe == t.Recipe {
		return nil
	}
	if t.Kind == KindProject {
		return fmt.Errorf("project %q carries the %s type label, and a project is never a Sketch: drop the label with `lym board upsert`", t.Slug, SketchType)
	}
	if prior != nil && prior.Kind == KindNote && prior.Discussed {
		return fmt.Errorf("task %q carries the %s type label with recipe %q, and a Sketch becomes a task only as a raddle run's entry; way forward: `lym board promote %s --recipe raddle`", t.Slug, SketchType, t.Recipe, t.Slug)
	}
	return fmt.Errorf("task %q carries the %s type label with recipe %q, and only a raddle run's entry is a Sketch made a task; way forward: `lym board upsert` setting the entry's recipe to raddle, or dropping the label", t.Slug, SketchType, t.Recipe)
}
